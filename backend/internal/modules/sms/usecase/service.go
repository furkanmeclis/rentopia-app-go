// Package usecase sends tenant SMS (TEC-734, docs/design/sms.md) through
// the integration job runner and issues / verifies SMS one-time codes.
//
//   - Send classes: otp and transactional go to the critical integrations
//     queue and never depend on marketing consent; informational sends
//     need the customer's open İYS SMS consent (customer_marketing_consents)
//     and are blocked otherwise.
//   - The log (sms_messages) holds a masked number, a keyed phone hash and
//     the body (OTP bodies redacted); the number and text the provider needs
//     are sealed (field cipher purpose "sms.payload") until the send is
//     final.
//   - Delivery reports come from the runner's CheckStatus (polling).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JobKind is the integration job kind of an SMS send.
const JobKind = "sms.send"

// Field cipher purposes.
const (
	PurposePayload   = "sms.payload"
	PurposePhoneHash = "sms.phone"
	PurposeBodyHash  = "sms.body"
	PurposeOTPCode   = "sms.otp.code"
)

// Activity resources.
const (
	ResourceOTP = "tenant.sms_otp"
	ResourceSMS = "tenant.sms"
)

// Message statuses (sms_messages.status).
const (
	StatusQueued      = "queued"
	StatusSubmitted   = "submitted"
	StatusDelivered   = "delivered"
	StatusUndelivered = "undelivered"
	StatusFailed      = "failed"
	StatusUnknown     = "unknown"
	StatusBlocked     = "blocked"
)

const maxBody = 900

var (
	// ErrNoAccount: no active SMS account is connected.
	ErrNoAccount = errors.New("no active SMS provider account is connected")
	// ErrNoConsent: an informational SMS without the customer's İYS consent.
	ErrNoConsent = errors.New("the customer has no SMS consent; informational messages are not sent")
	// ErrRateLimited: the organization's SMS volume limit is reached.
	ErrRateLimited = errors.New("SMS sending limit reached; try again later")
	// ErrAccountNotFound: unknown account (or another organization's).
	ErrAccountNotFound = errors.New("integration account not found")
)

// ValidationError is a field problem (400).
type ValidationError struct{ Field, Code, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Accounts is the part of the integration accounts service SMS needs.
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
	AccountByUUID(ctx context.Context, orgID int64, id uuid.UUID) (integrations.Account, error)
}

// Limits are the volume limits (DB counters over sms_messages /
// sms_otp_challenges, so they hold without Redis).
type Limits struct {
	// OrgPerHour caps the SMS of an organization per hour.
	OrgPerHour int64
	// OTPTTL is the code lifetime.
	OTPTTL time.Duration
	// OTPMaxAttempts is the number of wrong codes before the challenge locks.
	OTPMaxAttempts int32
	// OTPCooldown is the minimum time between two codes for one subject.
	OTPCooldown time.Duration
	// OTPPerPhoneHour / OTPPerOrgHour cap issued codes per number / org.
	OTPPerPhoneHour int64
	OTPPerOrgHour   int64
}

// DefaultLimits are the production defaults.
func DefaultLimits() Limits {
	return Limits{
		OrgPerHour: 1000, OTPTTL: 5 * time.Minute, OTPMaxAttempts: 5, OTPCooldown: 60 * time.Second,
		OTPPerPhoneHour: 5, OTPPerOrgHour: 300,
	}
}

// Service sends SMS and runs OTP challenges.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	cipher    *crypto.FieldCipher
	sealer    *crypto.Sealer
	accounts  Accounts
	runner    *jobsusecase.Runner
	providers map[string]sms.Provider
	activity  *activity.Recorder
	log       *slog.Logger
	limits    Limits
	now       func() time.Time
}

// New builds the service and registers the send operation of every
// provider on ops (the runner's operation registry).
func New(pool *pgxpool.Pool, q *db.Queries, fc *crypto.FieldCipher, accounts Accounts, runner *jobsusecase.Runner,
	ops *integrations.OperationRegistry, providers ...sms.Provider) (*Service, error) {
	if fc == nil {
		return nil, errors.New("sms: field cipher required")
	}
	sealer, err := fc.Sealer(PurposePayload)
	if err != nil {
		return nil, err
	}
	s := &Service{
		pool: pool, q: q, cipher: fc, sealer: sealer, accounts: accounts, runner: runner,
		providers: map[string]sms.Provider{}, log: slog.Default(), limits: DefaultLimits(), now: time.Now,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		s.providers[p.Provider()] = p
		ops.Register(p.Provider(), &sendOperation{svc: s, provider: p})
	}
	if runner != nil {
		runner.AddListener(s)
	}
	return s, nil
}

// SetActivity records OTP issue / verify and test sends.
func (s *Service) SetActivity(rec *activity.Recorder) { s.activity = rec }

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetLimits replaces the limits (tests).
func (s *Service) SetLimits(l Limits) { s.limits = l }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func orgScope(orgID int64) string { return "org:" + strconv.FormatInt(orgID, 10) }

func payloadAAD(orgID int64, id uuid.UUID) string {
	return "sms_messages.payload|" + orgScope(orgID) + "|message:" + id.String()
}

// PhoneHash is the keyed hash of a normalized number inside an organization.
func (s *Service) PhoneHash(orgID int64, normalized string) string {
	return s.cipher.BlindIndex(PurposePhoneHash, orgScope(orgID), normalized)
}

type sealedPayload struct {
	To    string `json:"to"`
	Body  string `json:"body"`
	Class string `json:"class"`
}

// SendInput is one SMS.
type SendInput struct {
	Class   string
	Purpose string
	// Phone in any common format; normalized and validated here.
	Phone string
	Body  string
	// RedactedBody is what the log keeps for an OTP (the code replaced).
	RedactedBody string
	// CustomerID: informational sends check this customer's consent.
	CustomerID *int64
	RefType    string
	RefUUID    *uuid.UUID
	// AccountUUID sends through this account instead of the active one
	// (connection test).
	AccountUUID *uuid.UUID
}

// Sent is a queued message.
type Sent struct {
	MessageUUID uuid.UUID `json:"message_uuid"`
	JobUUID     uuid.UUID `json:"job_uuid"`
	Status      string    `json:"status"`
	JobStatus   string    `json:"job_status"`
	PhoneMasked string    `json:"phone_masked"`
}

var purposeRE = regexp.MustCompile(`^[a-z][a-z0-9_.]{1,47}$`)

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

// NormalizePhone validates a number and returns its normalized form.
func NormalizePhone(v string) (string, error) {
	n := pii.Phone(v)
	if !pii.ValidPhone(n) {
		return "", &ValidationError{Field: "phone", Code: "invalid", Message: "a valid phone number is required"}
	}
	return n, nil
}

// Send queues one SMS through the organization's SMS account.
func (s *Service) Send(ctx context.Context, orgID int64, in SendInput) (Sent, error) {
	if !sms.ValidClass(in.Class) {
		return Sent{}, &ValidationError{Field: "class", Code: "invalid", Message: "unknown SMS class"}
	}
	if !purposeRE.MatchString(in.Purpose) {
		return Sent{}, &ValidationError{Field: "purpose", Code: "invalid", Message: "purpose must be a lower-case code"}
	}
	phone, err := NormalizePhone(in.Phone)
	if err != nil {
		return Sent{}, err
	}
	body := strings.TrimSpace(in.Body)
	if body == "" || len([]rune(body)) > maxBody {
		return Sent{}, &ValidationError{Field: "body", Code: "invalid", Message: "the text is required (at most 900 characters)"}
	}
	redacted := body
	if in.Class == sms.ClassOTP {
		redacted = strings.TrimSpace(in.RedactedBody)
		if redacted == "" || strings.Contains(redacted, body) {
			return Sent{}, errors.New("sms: an OTP needs a redacted body")
		}
	}
	var acc integrations.Account
	if in.AccountUUID != nil {
		acc, err = s.accounts.AccountByUUID(ctx, orgID, *in.AccountUUID)
		if err != nil {
			return Sent{}, ErrAccountNotFound
		}
	} else {
		acc, err = s.accounts.ActiveAccount(ctx, orgID, integrations.KindSMS)
		if err != nil {
			return Sent{}, ErrNoAccount
		}
	}
	if acc.Kind != integrations.KindSMS {
		return Sent{}, &ValidationError{Field: "account", Code: "invalid", Message: "the account is not an SMS account"}
	}
	if _, ok := s.providers[acc.Provider]; !ok {
		return Sent{}, ErrNoAccount
	}
	phoneHash := s.PhoneHash(orgID, phone)
	id := uuid.New()
	row := db.InsertSMSMessageParams{
		Uuid: id, OrganizationID: orgID, IntegrationAccountID: pgtype.Int8{Int64: acc.ID, Valid: true}, Class: in.Class,
		Purpose: in.Purpose, Provider: acc.Provider, PhoneMasked: pii.MaskPhone(phone), PhoneHash: phoneHash,
		BodyRedacted: redacted, BodyHash: s.cipher.BlindIndex(PurposeBodyHash, orgScope(orgID), redacted),
		Status: StatusQueued, RefType: in.RefType, CreatedBy: actorID(ctx),
	}
	if in.CustomerID != nil {
		row.CustomerID = pgtype.Int8{Int64: *in.CustomerID, Valid: true}
	}
	if in.RefUUID != nil {
		row.RefUuid = pgtype.UUID{Bytes: *in.RefUUID, Valid: true}
	}
	// İYS: informational messages need the customer's open SMS consent.
	if in.Class == sms.ClassInformational {
		ok := false
		if in.CustomerID != nil {
			ok, err = s.q.CustomerHasSMSConsent(ctx, db.CustomerHasSMSConsentParams{OrganizationID: orgID, CustomerID: *in.CustomerID})
			if err != nil {
				return Sent{}, err
			}
		}
		if !ok {
			row.Status, row.Error = StatusBlocked, "no İYS SMS consent"
			if _, err := s.q.InsertSMSMessage(ctx, row); err != nil {
				return Sent{}, err
			}
			return Sent{MessageUUID: id, Status: StatusBlocked, PhoneMasked: row.PhoneMasked}, ErrNoConsent
		}
	}
	if s.limits.OrgPerHour > 0 {
		n, err := s.q.CountSMSMessagesSince(ctx, db.CountSMSMessagesSinceParams{
			OrganizationID: orgID, Since: pgtype.Timestamptz{Time: s.now().Add(-time.Hour), Valid: true},
		})
		if err != nil {
			return Sent{}, err
		}
		if n >= s.limits.OrgPerHour {
			return Sent{}, ErrRateLimited
		}
	}
	raw, _ := json.Marshal(sealedPayload{To: phone, Body: body, Class: in.Class})
	row.PayloadCiphertext, err = s.sealer.Seal(string(raw), payloadAAD(orgID, id))
	if err != nil {
		return Sent{}, err
	}
	msg, err := s.q.InsertSMSMessage(ctx, row)
	if err != nil {
		return Sent{}, err
	}
	job, err := s.runner.Enqueue(ctx, jobsusecase.EnqueueInput{
		OrganizationID: orgID, AccountID: acc.ID, Provider: acc.Provider, Kind: JobKind,
		IdempotencyKey: fmt.Sprintf("org:%d:sms:%s", orgID, id),
		Payload:        map[string]any{"sms_message_uuid": id.String(), "class": in.Class, "purpose": in.Purpose, "to": row.PhoneMasked},
		RefType:        "sms_message", RefUUID: &id, Critical: in.Class != sms.ClassInformational, MaxAttempts: 8,
	})
	if err != nil {
		return Sent{}, err
	}
	if err := s.q.SetSMSMessageJob(ctx, db.SetSMSMessageJobParams{
		OrganizationID: orgID, ID: msg.ID, IntegrationJobID: pgtype.Int8{Int64: job.ID, Valid: true},
	}); err != nil {
		return Sent{}, err
	}
	out := Sent{MessageUUID: id, JobUUID: job.UUID, JobStatus: job.Status, PhoneMasked: row.PhoneMasked}
	if m, err := s.q.GetSMSMessage(ctx, db.GetSMSMessageParams{OrganizationID: orgID, ID: msg.ID}); err == nil {
		out.Status = m.Status
	}
	return out, nil
}

// AccountExists reports ErrAccountNotFound for an unknown account.
func (s *Service) AccountExists(ctx context.Context, orgID int64, id uuid.UUID) error {
	if _, err := s.accounts.AccountByUUID(ctx, orgID, id); err != nil {
		return ErrAccountNotFound
	}
	return nil
}

// TestSend sends a short test message through one account (panel
// "Test SMS"): transactional, logged like any other send.
func (s *Service) TestSend(ctx context.Context, orgID int64, account uuid.UUID, phone string) (Sent, error) {
	acc, err := s.accounts.AccountByUUID(ctx, orgID, account)
	if err != nil {
		return Sent{}, ErrAccountNotFound
	}
	if acc.Kind != integrations.KindSMS {
		return Sent{}, &ValidationError{Field: "account", Code: "invalid", Message: "the account is not an SMS account"}
	}
	return s.Send(ctx, orgID, SendInput{
		Class: sms.ClassTransactional, Purpose: "integration_test", Phone: phone,
		Body: "Rentopia SMS bağlantı testi. Bu mesajı aldıysanız SMS hesabınız çalışıyor.", AccountUUID: &account,
		RefType: "integration_account", RefUUID: &account,
	})
}

// IntegrationJobChanged implements the runner listener: the message follows
// its send job.
func (s *Service) IntegrationJobChanged(ctx context.Context, orgID int64, job jobsusecase.State, res integrations.Result) {
	if job.Kind != JobKind || orgID <= 0 {
		return
	}
	var p struct {
		Message string `json:"sms_message_uuid"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return
	}
	id, err := uuid.Parse(p.Message)
	if err != nil {
		return
	}
	status, clear := StatusQueued, false
	switch job.Status {
	case jobsusecase.StatusSubmitted:
		status = StatusSubmitted
	case jobsusecase.StatusSucceeded:
		status, clear = StatusDelivered, true
	case jobsusecase.StatusFailed:
		status, clear = StatusFailed, true
		if job.Operation == jobsusecase.OpCheckStatus {
			status = StatusUndelivered
		}
	case jobsusecase.StatusNeedsVerification:
		status = StatusUnknown
	}
	params := db.UpdateSMSMessageStatusParams{
		OrganizationID: orgID, Uuid: id, Status: status, IntegrationJobID: pgtype.Int8{Int64: job.ID, Valid: true},
		ProviderMessageID: job.ProviderRef, Error: job.LastError, ClearPayload: clear,
	}
	if code, ok := res.Response["dlr_code"].(string); ok {
		params.DlrStatus = code
	}
	if at, ok := res.Response["delivered_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, at); err == nil {
			params.DlrAt = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}
	if err := s.q.UpdateSMSMessageStatus(ctx, params); err != nil {
		s.log.Warn("sms_message_update_failed", "message", id, "error", err)
	}
}

// sendOperation adapts an sms.Provider to the runner's Operation contract.
type sendOperation struct {
	svc      *Service
	provider sms.Provider
}

func (o *sendOperation) Kind() string { return JobKind }

func (o *sendOperation) message(ctx context.Context, job integrations.Job) (db.GetSMSMessageForSendRow, sealedPayload, error) {
	var p struct {
		Message string `json:"sms_message_uuid"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return db.GetSMSMessageForSendRow{}, sealedPayload{}, err
	}
	id, err := uuid.Parse(p.Message)
	if err != nil {
		return db.GetSMSMessageForSendRow{}, sealedPayload{}, err
	}
	m, err := o.svc.q.GetSMSMessageForSend(ctx, db.GetSMSMessageForSendParams{OrganizationID: job.OrganizationID, Uuid: id})
	if err != nil {
		return m, sealedPayload{}, err
	}
	var sp sealedPayload
	if m.PayloadCiphertext == "" {
		return m, sp, errors.New("the message text is no longer available (final or expired)")
	}
	plain, err := o.svc.sealer.Open(m.PayloadCiphertext, payloadAAD(job.OrganizationID, m.Uuid))
	if err != nil {
		return m, sp, err
	}
	if err := json.Unmarshal([]byte(plain), &sp); err != nil {
		return m, sp, err
	}
	return m, sp, nil
}

// Submit implements integrations.Operation.
func (o *sendOperation) Submit(ctx context.Context, acc integrations.Account, job integrations.Job) (integrations.Result, error) {
	m, sp, err := o.message(ctx, job)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.ID == 0) {
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: "the message no longer exists"}, nil
	}
	if err != nil {
		// Nothing was sent: the provider was never called.
		return integrations.Result{Outcome: integrations.OutcomeFailed, Message: err.Error()}, nil
	}
	req := map[string]any{"to": pii.MaskPhone(sp.To), "class": sp.Class, "chars": len([]rune(sp.Body))}
	res, err := o.provider.Send(ctx, acc, sms.Message{To: sp.To, Body: sp.Body, Class: sp.Class})
	var rej *sms.Rejected
	switch {
	case errors.As(err, &rej):
		return integrations.Result{
			Outcome: integrations.OutcomeFailed, Retryable: rej.Retryable, Request: req,
			Response: map[string]any{"code": rej.Code}, Message: rej.Error(),
		}, nil
	case errors.Is(err, integrations.ErrUnsupported):
		return integrations.Result{Outcome: integrations.OutcomeUnsupported, Request: req, Message: err.Error()}, err
	case err != nil:
		return integrations.Uncertain(err, req)
	}
	return integrations.Result{
		Outcome: integrations.OutcomeAccepted, ProviderRef: res.MessageID, Request: req,
		Response: map[string]any{"code": res.Code, "message_id": res.MessageID},
	}, nil
}

// CheckStatus implements integrations.Operation: the delivery report.
func (o *sendOperation) CheckStatus(ctx context.Context, acc integrations.Account, job integrations.Job) (integrations.Result, error) {
	req := map[string]any{"message_id": job.ProviderRef}
	if job.ProviderRef == "" {
		// The provider never answered with a reference: Netgsm cannot look
		// the message up, so a person decides (never resent blindly).
		return integrations.Result{Request: req}, fmt.Errorf("%w: no provider message id to look up", integrations.ErrUnsupported)
	}
	d, err := o.provider.Status(ctx, acc, job.ProviderRef)
	if errors.Is(err, integrations.ErrUnsupported) {
		return integrations.Result{Request: req}, err
	}
	if err != nil {
		return integrations.Uncertain(err, req)
	}
	resp := map[string]any{"state": string(d.State), "dlr_code": d.Code}
	if d.At != nil {
		resp["delivered_at"] = d.At.UTC().Format(time.RFC3339)
	}
	switch d.State {
	case sms.DeliveryDelivered:
		return integrations.Result{Outcome: integrations.OutcomeSucceeded, ProviderRef: job.ProviderRef, Request: req, Response: resp}, nil
	case sms.DeliveryUndelivered:
		return integrations.Result{
			Outcome: integrations.OutcomeFailed, Request: req, Response: resp, Message: "not delivered (report code " + d.Code + ")",
		}, nil
	case sms.DeliveryNotFound:
		// The provider accepted the message with this id earlier: a missing
		// report is "not yet", never "not sent".
		return integrations.Result{Outcome: integrations.OutcomePending, Request: req, Response: resp, Message: "no report yet"}, nil
	default:
		return integrations.Result{Outcome: integrations.OutcomePending, Request: req, Response: resp}, nil
	}
}

// audit writes an activity entry (never a code or a full number).
func (s *Service) audit(ctx context.Context, orgID int64, action, resource string, id uuid.UUID, payload map[string]any) {
	if s.activity == nil {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	var actor *int64
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		v := p.UserInternal
		actor = &v
	}
	s.activity.Record(ctx, actor, action, resource, &id, payload, nil)
}
