package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Remote approval (TEC-711): staff create a time-limited link for the
// current version of a document; the customer opens /d/{token}, receives a
// one-time SMS code on the number staff chose and approves exactly that
// version and content hash. docs/design/contracts.md §7.

// PurposeApprovalLink is the field cipher purpose of the sealed link token
// and number snapshot.
const PurposeApprovalLink = "documents.approval_link"

// SMS purposes of remote approval.
const smsPurposeLink = "contract_approval_link"

var (
	// ErrRemoteUnavailable: remote approval is not wired (no SMS service or
	// cipher in this process).
	ErrRemoteUnavailable = errors.New("remote approval is not available")
	// ErrLinkNotFound: unknown token / link (or another organization's).
	ErrLinkNotFound = errors.New("approval link not found")
	// ErrCodeInvalid: a code that does not verify (wrong, expired, locked,
	// never sent): one generic answer.
	ErrCodeInvalid = errors.New("the code could not be verified; check it or request a new code")
	// ErrRemoteRateLimited: a per-link limit is reached.
	ErrRemoteRateLimited = errors.New("too many attempts; try again later")
	// ErrCodeUnavailable: the code could not be sent (generic).
	ErrCodeUnavailable = errors.New("the code could not be sent; try again later or contact the office")
	// ErrNoSMSAccount: the organization has no active SMS account.
	ErrNoSMSAccount = errors.New("no active SMS account is connected")
	// ErrSMSRateLimited: the organization's SMS limit is reached.
	ErrSMSRateLimited = errors.New("SMS sending limit reached; try again later")
)

// LinkStateError: the link can no longer be used (409 LINK_UNAVAILABLE).
type LinkStateError struct{ State string }

func (e *LinkStateError) Error() string { return "this approval link is " + e.State }

// CooldownError: a code was sent a moment ago (429 with Retry-After).
type CooldownError struct{ RetryAfter time.Duration }

func (e *CooldownError) Error() string { return "a code was just sent; try again shortly" }

// RemoteSMS is the SMS / OTP service remote approval uses.
type RemoteSMS interface {
	IssueOTP(ctx context.Context, orgID int64, purpose, subjectRef, phone string) (smsusecase.Challenge, error)
	VerifyOTP(ctx context.Context, orgID int64, purpose, subjectRef, code string) (smsusecase.Verification, error)
	Send(ctx context.Context, orgID int64, in smsusecase.SendInput) (smsusecase.Sent, error)
	PhoneHash(orgID int64, normalized string) string
	HasActiveAccount(ctx context.Context, orgID int64) bool
}

// RemoteLimits bound links and their per-link counters (DB counters: they
// hold without Redis; the handler adds per-IP / per-token windows).
type RemoteLimits struct {
	DefaultTTL, MinTTL, MaxTTL time.Duration
	// MaxCodesPerLink: SMS codes one link may request in its lifetime.
	MaxCodesPerLink int32
	// MaxFailuresPerLink: wrong codes before the link stops accepting codes.
	MaxFailuresPerLink int32
}

// DefaultRemoteLimits are the production limits.
func DefaultRemoteLimits() RemoteLimits {
	return RemoteLimits{
		DefaultTTL: 24 * time.Hour, MinTTL: 15 * time.Minute, MaxTTL: 7 * 24 * time.Hour,
		MaxCodesPerLink: 10, MaxFailuresPerLink: 15,
	}
}

// Remote wires remote approval.
type Remote struct {
	SMS    RemoteSMS
	Cipher *crypto.FieldCipher
	// PublicBaseURL is the browser origin of /d/{token} (the Next.js app).
	PublicBaseURL string
	Activity      *activity.Recorder
}

type remote struct {
	sms      RemoteSMS
	sealer   *crypto.Sealer
	baseURL  string
	activity *activity.Recorder
	limits   RemoteLimits
}

// SetRemote enables remote approval.
func (s *Service) SetRemote(r Remote) error {
	if r.SMS == nil || r.Cipher == nil {
		return errors.New("contracts: remote approval needs the SMS service and the field cipher")
	}
	sealer, err := r.Cipher.Sealer(PurposeApprovalLink)
	if err != nil {
		return err
	}
	s.remote = &remote{
		sms: r.SMS, sealer: sealer, baseURL: strings.TrimRight(r.PublicBaseURL, "/"), activity: r.Activity,
		limits: DefaultRemoteLimits(),
	}
	return nil
}

// SetRemoteLimits replaces the limits (tests).
func (s *Service) SetRemoteLimits(l RemoteLimits) {
	if s.remote != nil {
		s.remote.limits = l
	}
}

// ------------------------------------------------------------- helpers

var linkTokenRE = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// newToken is 32 random bytes, base64url (43 characters).
func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// tokenHash is the lookup key of a token (SHA-256 hex; the token has 256
// bits of entropy, so no key is needed).
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokenKey is the rate-limit subject of a token (never the token itself).
func TokenKey(token string) string { return tokenHash(token)[:32] }

func linkAAD(field string, orgID int64, link uuid.UUID) string {
	return "document_approval_links." + field + "|org:" + strconv.FormatInt(orgID, 10) + "|link:" + link.String()
}

func otpSubject(version uuid.UUID) string { return "document_version:" + version.String() }

func (r *remote) url(token string) string { return r.baseURL + "/d/" + token }

// effectiveLinkStatus applies the expiry of a pending link lazily.
func effectiveLinkStatus(status string, expires pgtype.Timestamptz, now time.Time) string {
	if status == model.LinkPending && expires.Valid && !now.Before(expires.Time) {
		return model.LinkExpired
	}
	return status
}

func customerName(customerType, first, last, company string) string {
	if customerType == "corporate" && strings.TrimSpace(company) != "" {
		return strings.TrimSpace(company)
	}
	return strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
}

// shortName keeps the first name and the initial of the last (the public
// page names the signer without the full surname).
func shortName(full string) string {
	parts := strings.Fields(full)
	if len(parts) < 2 {
		return full
	}
	last := []rune(parts[len(parts)-1])
	return strings.Join(parts[:len(parts)-1], " ") + " " + string(last[0]) + "."
}

func (s *Service) remoteOn() (*remote, error) {
	if s.remote == nil {
		return nil, ErrRemoteUnavailable
	}
	return s.remote, nil
}

// requestFor builds the request the activity recorder reads the IP address
// and the user agent from.
func requestFor(c model.ClientInfo) *http.Request {
	r := &http.Request{Header: http.Header{}, RemoteAddr: net.JoinHostPort(c.IP, "0")}
	if c.UserAgent != "" {
		r.Header.Set("User-Agent", c.UserAgent)
	}
	return r
}

func (r *remote) audit(ctx context.Context, orgID int64, action string, docID uuid.UUID, payload map[string]any, c *model.ClientInfo) {
	if r.activity == nil {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	var req *http.Request
	if c != nil {
		req = requestFor(*c)
	}
	r.activity.Record(ctx, actorID(ctx), action, model.ResourceDocuments, &docID, payload, req)
}

// ------------------------------------------------------------- signers

type signer struct {
	role       string
	name       string
	phone      string
	customerID pgtype.Int8
	driverID   pgtype.Int8
}

func (s *Service) signerOptions(ctx context.Context, orgID int64, doc db.GetDocumentRow) ([]model.Signer, map[string]signer, error) {
	out := []model.Signer{}
	byKey := map[string]signer{}
	add := func(sg signer, driverUUID *uuid.UUID) {
		phone := pii.Phone(sg.phone)
		valid := pii.ValidPhone(phone)
		ms := model.Signer{Role: sg.role, DriverUUID: driverUUID, Name: sg.name, HasPhone: valid}
		if valid {
			ms.PhoneMasked = pii.MaskPhone(phone)
			sg.phone = phone
		} else {
			sg.phone = ""
		}
		key := sg.role
		if driverUUID != nil {
			key += ":" + driverUUID.String()
		}
		byKey[key] = sg
		out = append(out, ms)
	}
	renter, err := s.q.GetDocumentSignerRenter(ctx, db.GetDocumentSignerRenterParams{OrganizationID: orgID, ReservationID: doc.ReservationID})
	if err != nil && !notFound(err) {
		return nil, nil, err
	}
	if err == nil {
		add(signer{
			role: model.SignerRenter, name: customerName(renter.CustomerType, renter.FirstName, renter.LastName, renter.CompanyTitle),
			phone: renter.Phone, customerID: pgtype.Int8{Int64: renter.ID, Valid: true},
		}, nil)
	}
	drivers, err := s.q.ListDocumentSignerDrivers(ctx, db.ListDocumentSignerDriversParams{OrganizationID: orgID, ReservationID: doc.ReservationID})
	if err != nil {
		return nil, nil, err
	}
	for _, d := range drivers {
		id := d.Uuid
		add(signer{
			role: model.SignerDriver, name: strings.TrimSpace(d.FirstName + " " + d.LastName), phone: d.Phone,
			customerID: pgtype.Int8{Int64: d.CustomerID, Valid: true}, driverID: pgtype.Int8{Int64: d.ID, Valid: true},
		}, &id)
	}
	return out, byKey, nil
}

// Signers lists who can approve a document remotely (renter and drivers,
// masked numbers) and whether SMS is available.
func (s *Service) Signers(ctx context.Context, orgID int64, docID uuid.UUID) (model.SignerOptions, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.SignerOptions{}, err
	}
	list, _, err := s.signerOptions(ctx, orgID, doc)
	if err != nil {
		return model.SignerOptions{}, err
	}
	out := model.SignerOptions{Signers: list}
	if s.remote != nil {
		out.SMSAvailable = s.remote.sms.HasActiveAccount(ctx, orgID)
	}
	return out, nil
}

// ------------------------------------------------------------- staff

func (s *Service) linkView(r *remote, orgID int64, row db.ListDocumentApprovalLinksRow, now time.Time) model.ApprovalLink {
	l := model.ApprovalLink{
		UUID: row.Uuid, VersionUUID: row.VersionUuid, VersionNumber: row.VersionNumber, ContentHash: row.ContentHash,
		Status: effectiveLinkStatus(row.Status, row.ExpiresAt, now), SignerRole: row.SignerRole, SignerName: row.SignerName,
		PhoneMasked: row.PhoneMasked, ExpiresAt: row.ExpiresAt.Time.UTC(), CodesSent: row.CodesSent,
		FailedAttempts: row.FailedAttempts, SMSSentCount: row.SmsSentCount, LastSMSAt: timePtr(row.LastSmsAt),
		ApprovedAt: timePtr(row.ApprovedAt), RevokedAt: timePtr(row.RevokedAt), RevokedReason: row.RevokedReason,
		CreatedBy: actorOf(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		CreatedAt: row.CreatedAt.Time.UTC(),
	}
	if row.ApprovalUuid.Valid {
		a := uuid.UUID(row.ApprovalUuid.Bytes)
		l.ApprovalUUID = &a
	}
	if l.Status == model.LinkPending && r != nil {
		if token, err := r.sealer.Open(row.TokenCiphertext, linkAAD("token", orgID, row.Uuid)); err == nil {
			l.URL = r.url(token)
		}
	}
	return l
}

// ListApprovalLinks returns the remote approval links of a document.
func (s *Service) ListApprovalLinks(ctx context.Context, orgID int64, docID uuid.UUID) ([]model.ApprovalLink, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListDocumentApprovalLinks(ctx, db.ListDocumentApprovalLinksParams{OrganizationID: orgID, DocumentID: doc.ID})
	if err != nil {
		return nil, err
	}
	now := s.clock()
	out := make([]model.ApprovalLink, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.linkView(s.remote, orgID, row, now))
	}
	return out, nil
}

func (s *Service) oneLink(ctx context.Context, orgID, docID int64, id uuid.UUID) (model.ApprovalLink, error) {
	rows, err := s.q.ListDocumentApprovalLinks(ctx, db.ListDocumentApprovalLinksParams{OrganizationID: orgID, DocumentID: docID})
	if err != nil {
		return model.ApprovalLink{}, err
	}
	for _, row := range rows {
		if row.Uuid == id {
			return s.linkView(s.remote, orgID, row, s.clock()), nil
		}
	}
	return model.ApprovalLink{}, ErrLinkNotFound
}

// CreateApprovalLink creates the remote approval link of a version (the
// one staff showed: content_hash must match). The document's open link
// is replaced. send_sms also sends the link by SMS (transactional).
func (s *Service) CreateApprovalLink(ctx context.Context, orgID int64, docID, versionID uuid.UUID, in model.CreateLinkInput) (model.CreatedLink, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.CreatedLink{}, err
	}
	v, err := s.versionRow(ctx, orgID, doc, versionID)
	if err != nil {
		return model.CreatedLink{}, err
	}
	if !canWrite(ctx) {
		return model.CreatedLink{}, ErrForbidden
	}
	r, err := s.remoteOn()
	if err != nil {
		return model.CreatedLink{}, err
	}
	ve := &ValidationError{}
	hash := strings.ToLower(strings.TrimSpace(in.ContentHash))
	if hash == "" {
		ve.add("content_hash", "required", "content_hash of the version shown is required")
	}
	key := in.SignerRole
	switch in.SignerRole {
	case model.SignerRenter:
	case model.SignerDriver:
		if in.DriverUUID == nil {
			ve.add("driver_uuid", "required", "driver_uuid is required for a driver")
		} else {
			key += ":" + in.DriverUUID.String()
		}
	default:
		ve.add("signer_role", "invalid", "signer_role is renter or driver")
	}
	ttl := r.limits.DefaultTTL
	if in.ExpiresInMinutes != nil {
		ttl = time.Duration(*in.ExpiresInMinutes) * time.Minute
		if ttl < r.limits.MinTTL || ttl > r.limits.MaxTTL {
			ve.add("expires_in_minutes", "out_of_range", fmt.Sprintf("expires_in_minutes is %d-%d",
				int(r.limits.MinTTL.Minutes()), int(r.limits.MaxTTL.Minutes())))
		}
	}
	if err := ve.orNil(); err != nil {
		return model.CreatedLink{}, err
	}
	if doc.Status == model.DocumentVoid {
		return model.CreatedLink{}, stateErr("void", "the document was withdrawn")
	}
	if !doc.CurrentVersionID.Valid || doc.CurrentVersionID.Int64 != v.ID || v.Status == model.VersionSuperseded {
		return model.CreatedLink{}, &ContentChangedError{Current: currentUUID(ctx, s, orgID, doc)}
	}
	if hash != v.ContentHash {
		return model.CreatedLink{}, &ContentChangedError{Current: &v.Uuid}
	}
	if v.Status == model.VersionApproved {
		return model.CreatedLink{}, stateErr("already_approved", "this version is already approved")
	}
	_, signers, err := s.signerOptions(ctx, orgID, doc)
	if err != nil {
		return model.CreatedLink{}, err
	}
	sg, ok := signers[key]
	if !ok {
		field := "signer_role"
		if in.SignerRole == model.SignerDriver {
			field = "driver_uuid"
		}
		return model.CreatedLink{}, validation(field, "not_found", "the person is not on this reservation")
	}
	if sg.phone == "" {
		return model.CreatedLink{}, validation("signer_role", "no_phone", "the person has no valid mobile number")
	}
	token, err := newToken()
	if err != nil {
		return model.CreatedLink{}, err
	}
	id := uuid.New()
	sealedToken, err := r.sealer.Seal(token, linkAAD("token", orgID, id))
	if err != nil {
		return model.CreatedLink{}, err
	}
	sealedPhone, err := r.sealer.Seal(sg.phone, linkAAD("phone", orgID, id))
	if err != nil {
		return model.CreatedLink{}, err
	}
	now := s.clock()
	err = s.tx(ctx, func(q *db.Queries) error {
		if err := q.ExpireDocumentApprovalLinks(ctx, db.ExpireDocumentApprovalLinksParams{OrganizationID: orgID, DocumentID: doc.ID, Now: ts(now)}); err != nil {
			return err
		}
		if _, err := q.CloseOpenDocumentApprovalLinks(ctx, db.CloseOpenDocumentApprovalLinksParams{
			Status: model.LinkRevoked, ClosedAt: ts(now), RevokedBy: int8Ptr(actorID(ctx)), Reason: "replaced",
			OrganizationID: orgID, DocumentID: doc.ID,
		}); err != nil {
			return err
		}
		_, err := q.InsertDocumentApprovalLink(ctx, db.InsertDocumentApprovalLinkParams{
			Uuid: id, OrganizationID: orgID, DocumentID: doc.ID, DocumentVersionID: v.ID, ContentHash: v.ContentHash,
			TokenHash: tokenHash(token), TokenCiphertext: sealedToken, SignerRole: sg.role, SignerName: sg.name,
			CustomerID: sg.customerID, DriverID: sg.driverID, PhoneMasked: pii.MaskPhone(sg.phone),
			PhoneHash: r.sms.PhoneHash(orgID, sg.phone), PhoneCiphertext: sealedPhone, ExpiresAt: ts(now.Add(ttl)),
			CreatedBy: int8Ptr(actorID(ctx)),
		})
		return err
	})
	if uniqueViolation(err, "uq_document_approval_links_pending") {
		return model.CreatedLink{}, stateErr("concurrent_link", "another link was created at the same time; reload")
	}
	if err != nil {
		return model.CreatedLink{}, err
	}
	s.publish(ctx, model.ResourceDocuments, "updated", doc.Uuid)
	link, err := s.oneLink(ctx, orgID, doc.ID, id)
	if err != nil {
		return model.CreatedLink{}, err
	}
	out := model.CreatedLink{Link: link}
	if in.SendSMS {
		sent, err := s.sendLink(ctx, r, orgID, doc, id, token, sg.phone, ttl, now)
		if err != nil {
			out.SMSError = err.Error()
		} else {
			out.SMS = &sent
			if l, err := s.oneLink(ctx, orgID, doc.ID, id); err == nil {
				out.Link, out.SMS.Link = l, l
			}
		}
	}
	return out, nil
}

func linkSMSBody(org, title, reference, url string, until time.Time) string {
	return fmt.Sprintf("%s: %s (%s) onayınızı bekliyor. Belgeyi okuyup SMS koduyla onaylamak için: %s Son geçerlilik: %s",
		org, title, reference, url, rentaltime.Local(until).Format("02.01.2006 15:04"))
}

// sendLink sends the link by SMS: transactional class (a notice about the
// contract the customer is entering; no marketing consent needed). The
// log keeps the text with the token redacted.
func (s *Service) sendLink(ctx context.Context, r *remote, orgID int64, doc db.GetDocumentRow, linkID uuid.UUID, token, phone string,
	ttl time.Duration, created time.Time) (model.LinkSMS, error) {
	org, err := s.q.GetDocumentOrganization(ctx, orgID)
	if err != nil {
		return model.LinkSMS{}, err
	}
	title := kindTitles[doc.Kind]
	until := created.Add(ttl)
	body := linkSMSBody(org.Name, title, doc.ReservationReference, r.url(token), until)
	redacted := linkSMSBody(org.Name, title, doc.ReservationReference, r.baseURL+"/d/••••••", until)
	sent, err := r.sms.Send(ctx, orgID, smsusecase.SendInput{
		Class: sms.ClassTransactional, Purpose: smsPurposeLink, Phone: phone, Body: body, RedactedBody: redacted,
		RefType: "document_approval_link", RefUUID: &linkID,
	})
	switch {
	case errors.Is(err, smsusecase.ErrNoAccount):
		return model.LinkSMS{}, ErrNoSMSAccount
	case errors.Is(err, smsusecase.ErrRateLimited):
		return model.LinkSMS{}, ErrSMSRateLimited
	case err != nil:
		return model.LinkSMS{}, err
	}
	id, err := s.q.GetDocumentApprovalLink(ctx, db.GetDocumentApprovalLinkParams{OrganizationID: orgID, DocumentID: doc.ID, Uuid: linkID})
	if err == nil {
		_ = s.q.MarkDocumentApprovalLinkSMS(ctx, db.MarkDocumentApprovalLinkSMSParams{OrganizationID: orgID, ID: id.ID, SentAt: ts(s.clock())})
	}
	return model.LinkSMS{MessageUUID: sent.MessageUUID, PhoneMasked: sent.PhoneMasked}, nil
}

func (s *Service) linkRow(ctx context.Context, orgID int64, doc db.GetDocumentRow, id uuid.UUID) (db.GetDocumentApprovalLinkRow, error) {
	row, err := s.q.GetDocumentApprovalLink(ctx, db.GetDocumentApprovalLinkParams{OrganizationID: orgID, DocumentID: doc.ID, Uuid: id})
	if notFound(err) {
		return row, ErrLinkNotFound
	}
	return row, err
}

// SendApprovalLink (re)sends an open link by SMS.
func (s *Service) SendApprovalLink(ctx context.Context, orgID int64, docID, linkID uuid.UUID) (model.LinkSMS, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.LinkSMS{}, err
	}
	row, err := s.linkRow(ctx, orgID, doc, linkID)
	if err != nil {
		return model.LinkSMS{}, err
	}
	if !canWrite(ctx) {
		return model.LinkSMS{}, ErrForbidden
	}
	r, err := s.remoteOn()
	if err != nil {
		return model.LinkSMS{}, err
	}
	now := s.clock()
	if effectiveLinkStatus(row.Status, row.ExpiresAt, now) != model.LinkPending {
		return model.LinkSMS{}, stateErr("link_not_pending", "only an open link can be sent")
	}
	token, err := r.sealer.Open(row.TokenCiphertext, linkAAD("token", orgID, row.Uuid))
	if err != nil {
		return model.LinkSMS{}, err
	}
	phone, err := r.sealer.Open(row.PhoneCiphertext, linkAAD("phone", orgID, row.Uuid))
	if err != nil {
		return model.LinkSMS{}, err
	}
	sent, err := s.sendLink(ctx, r, orgID, doc, row.Uuid, token, phone, row.ExpiresAt.Time.Sub(now), now)
	if err != nil {
		return model.LinkSMS{}, err
	}
	sent.Link, err = s.oneLink(ctx, orgID, doc.ID, row.Uuid)
	s.publish(ctx, model.ResourceDocuments, "updated", doc.Uuid)
	return sent, err
}

// RevokeApprovalLink closes an open link.
func (s *Service) RevokeApprovalLink(ctx context.Context, orgID int64, docID, linkID uuid.UUID) (model.ApprovalLink, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.ApprovalLink{}, err
	}
	row, err := s.linkRow(ctx, orgID, doc, linkID)
	if err != nil {
		return model.ApprovalLink{}, err
	}
	if !canWrite(ctx) {
		return model.ApprovalLink{}, ErrForbidden
	}
	n, err := s.q.RevokeDocumentApprovalLink(ctx, db.RevokeDocumentApprovalLinkParams{
		OrganizationID: orgID, ID: row.ID, RevokedAt: ts(s.clock()), RevokedBy: int8Ptr(actorID(ctx)), Reason: "revoked",
	})
	if err != nil {
		return model.ApprovalLink{}, err
	}
	if n != 1 {
		return model.ApprovalLink{}, stateErr("link_not_pending", "only an open link can be revoked")
	}
	s.publish(ctx, model.ResourceDocuments, "updated", doc.Uuid)
	return s.oneLink(ctx, orgID, doc.ID, row.Uuid)
}

// ------------------------------------------------------------- public

// resolveLink finds a link by its public token (organization included).
func (s *Service) resolveLink(ctx context.Context, token string) (context.Context, int64, db.GetDocumentApprovalLinkPublicRow, error) {
	var row db.GetDocumentApprovalLinkPublicRow
	if s.remote == nil {
		return ctx, 0, row, ErrLinkNotFound
	}
	token = strings.TrimSpace(token)
	if !linkTokenRE.MatchString(token) {
		return ctx, 0, row, ErrLinkNotFound
	}
	ref, err := s.q.ResolveDocumentApprovalLinkToken(ctx, tokenHash(token))
	if notFound(err) {
		return ctx, 0, row, ErrLinkNotFound
	}
	if err != nil {
		return ctx, 0, row, err
	}
	octx := orgctx.WithOrganizationID(ctx, ref.OrganizationID)
	row, err = s.q.GetDocumentApprovalLinkPublic(octx, db.GetDocumentApprovalLinkPublicParams(ref))
	if notFound(err) {
		return ctx, 0, row, ErrLinkNotFound
	}
	return octx, ref.OrganizationID, row, err
}

// publicState is the state the page shows.
func publicState(row db.GetDocumentApprovalLinkPublicRow, now time.Time) string {
	switch row.Status {
	case model.LinkApproved, model.LinkRevoked, model.LinkSuperseded, model.LinkExpired:
		return row.Status
	}
	switch {
	case row.DocumentStatus == model.DocumentVoid:
		return model.LinkRevoked
	case row.VersionStatus == model.VersionSuperseded || !row.CurrentVersionID.Valid || row.CurrentVersionID.Int64 != row.DocumentVersionID:
		return model.LinkSuperseded
	case !now.Before(row.ExpiresAt.Time):
		return model.LinkExpired
	}
	return model.LinkPending
}

func publicView(row db.GetDocumentApprovalLinkPublicRow, state string) model.PublicDocument {
	out := model.PublicDocument{
		State: state, OrganizationName: row.OrganizationName, Kind: row.Kind, Reference: row.ReservationReference,
		VersionNumber: row.VersionNumber, ContentHash: row.ContentHash, SignerName: shortName(row.SignerName),
		ExpiresAt: row.ExpiresAt.Time.UTC(), ApprovedAt: timePtr(row.ApprovedAt),
	}
	switch state {
	case model.LinkPending:
		out.HTML, out.PhoneMasked = row.Html, row.PhoneMasked
	case model.LinkApproved:
		out.HTML, out.PDFAvailable = row.Html, true
	}
	return out
}

// PublicDocument answers the public approval page.
func (s *Service) PublicDocument(ctx context.Context, token string) (model.PublicDocument, error) {
	octx, orgID, row, err := s.resolveLink(ctx, token)
	if err != nil {
		return model.PublicDocument{}, err
	}
	state := publicState(row, s.clock())
	if state == model.LinkExpired && row.Status == model.LinkPending {
		_ = s.q.MarkDocumentApprovalLinkExpired(octx, db.MarkDocumentApprovalLinkExpiredParams{OrganizationID: orgID, ID: row.ID})
	}
	return publicView(row, state), nil
}

func usable(row db.GetDocumentApprovalLinkPublicRow, state string) error {
	switch state {
	case model.LinkPending:
		return nil
	case model.LinkSuperseded:
		// The content changed since the page was loaded.
		return &ContentChangedError{}
	}
	return &LinkStateError{State: state}
}

// RequestCode sends a one-time code to the link's number (purpose
// contract_approval, subject document_version:<uuid>). Errors are generic:
// nothing tells whether a number exists or was reachable.
func (s *Service) RequestCode(ctx context.Context, token string, client model.ClientInfo) (model.PublicCode, error) {
	octx, orgID, row, err := s.resolveLink(ctx, token)
	if err != nil {
		return model.PublicCode{}, err
	}
	r := s.remote
	if err := usable(row, publicState(row, s.clock())); err != nil {
		return model.PublicCode{}, err
	}
	if row.CodesSent >= r.limits.MaxCodesPerLink || row.FailedAttempts >= r.limits.MaxFailuresPerLink {
		return model.PublicCode{}, ErrRemoteRateLimited
	}
	phone, err := r.sealer.Open(row.PhoneCiphertext, linkAAD("phone", orgID, row.Uuid))
	if err != nil {
		return model.PublicCode{}, err
	}
	ch, err := r.sms.IssueOTP(octx, orgID, model.OTPPurpose, otpSubject(row.VersionUuid), phone)
	var cd *smsusecase.CooldownError
	switch {
	case errors.As(err, &cd):
		return model.PublicCode{}, &CooldownError{RetryAfter: cd.RetryAfter}
	case errors.Is(err, smsusecase.ErrOTPRateLimited), errors.Is(err, smsusecase.ErrRateLimited):
		return model.PublicCode{}, ErrRemoteRateLimited
	case err != nil:
		s.log.Warn("remote_approval_code_failed", "organization_id", orgID, "link", row.Uuid, "error", err)
		return model.PublicCode{}, ErrCodeUnavailable
	}
	if _, err := s.q.CountDocumentApprovalLinkCode(octx, db.CountDocumentApprovalLinkCodeParams{
		OrganizationID: orgID, ID: row.ID, MaxCodes: r.limits.MaxCodesPerLink,
	}); err != nil {
		return model.PublicCode{}, err
	}
	r.audit(octx, orgID, "documents.remote_code_sent", row.DocumentUuid, map[string]any{
		"link_uuid": row.Uuid, "version_uuid": row.VersionUuid, "phone": row.PhoneMasked, "challenge_uuid": ch.UUID,
	}, &client)
	return model.PublicCode{PhoneMasked: ch.PhoneMasked, ExpiresAt: ch.ExpiresAt, ResendAfter: ch.ResendAfter}, nil
}

func clientIP(v string) *netip.Addr {
	a, err := netip.ParseAddr(strings.TrimSpace(v))
	if err != nil {
		return nil
	}
	return &a
}

// clip cuts v to at most n bytes (a rune cut in half is dropped).
func clip(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return strings.ToValidUTF8(v[:n], "")
}

// ApproveRemote verifies the code and records the approval of the link's
// exact version and content hash (the hash the page displayed must match;
// a version superseded since answers CONTENT_CHANGED).
func (s *Service) ApproveRemote(ctx context.Context, token string, in model.PublicApproveInput, client model.ClientInfo) (model.PublicDocument, error) {
	octx, orgID, row, err := s.resolveLink(ctx, token)
	if err != nil {
		return model.PublicDocument{}, err
	}
	r := s.remote
	if err := usable(row, publicState(row, s.clock())); err != nil {
		return model.PublicDocument{}, err
	}
	if row.FailedAttempts >= r.limits.MaxFailuresPerLink {
		return model.PublicDocument{}, ErrRemoteRateLimited
	}
	hash := strings.ToLower(strings.TrimSpace(in.ContentHash))
	if hash != row.ContentHash {
		return model.PublicDocument{}, &ContentChangedError{}
	}
	fail := func() error {
		_ = s.q.RecordDocumentApprovalLinkFailure(octx, db.RecordDocumentApprovalLinkFailureParams{OrganizationID: orgID, ID: row.ID})
		r.audit(octx, orgID, "documents.remote_approval_failed", row.DocumentUuid, map[string]any{
			"link_uuid": row.Uuid, "version_uuid": row.VersionUuid,
		}, &client)
		return ErrCodeInvalid
	}
	ver, err := r.sms.VerifyOTP(octx, orgID, model.OTPPurpose, otpSubject(row.VersionUuid), in.Code)
	if err != nil {
		var ve *smsusecase.ValidationError
		if errors.Is(err, smsusecase.ErrOTPInvalid) || errors.Is(err, smsusecase.ErrOTPNotFound) || errors.Is(err, smsusecase.ErrOTPExpired) ||
			errors.Is(err, smsusecase.ErrOTPLocked) || errors.As(err, &ve) {
			return model.PublicDocument{}, fail()
		}
		return model.PublicDocument{}, err
	}
	// The code must have gone to this link's number after the link existed.
	bind, err := s.q.GetOTPChallengeBinding(octx, db.GetOTPChallengeBindingParams{OrganizationID: orgID, Uuid: ver.ChallengeUUID})
	if err != nil || bind.PhoneHash != row.PhoneHash || bind.CreatedAt.Time.Before(row.CreatedAt.Time) {
		return model.PublicDocument{}, fail()
	}
	now := s.clock()
	ua := clip(strings.TrimSpace(client.UserAgent), 512)
	var approvalUUID uuid.UUID
	action := "document_approved"
	if row.Kind == model.KindTerms {
		action = "terms_approved"
	}
	err = s.tx(octx, func(q *db.Queries) error {
		d, err := q.LockDocument(octx, db.LockDocumentParams{OrganizationID: orgID, ID: row.DocumentID})
		if err != nil {
			return err
		}
		if d.Status == model.DocumentVoid {
			return &LinkStateError{State: model.LinkRevoked}
		}
		if !d.CurrentVersionID.Valid || d.CurrentVersionID.Int64 != row.DocumentVersionID {
			return &ContentChangedError{}
		}
		l, err := q.LockDocumentApprovalLink(octx, db.LockDocumentApprovalLinkParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		switch {
		case l.Status == model.LinkSuperseded:
			return &ContentChangedError{}
		case l.Status != model.LinkPending:
			return &LinkStateError{State: l.Status}
		case !now.Before(l.ExpiresAt.Time):
			return &LinkStateError{State: model.LinkExpired}
		}
		n, err := q.ApproveDocumentVersion(octx, db.ApproveDocumentVersionParams{
			OrganizationID: orgID, ID: row.DocumentVersionID, ContentHash: hash, ApprovedAt: ts(now),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return &ContentChangedError{}
		}
		approvalUUID, err = q.InsertRemoteDocumentApproval(octx, db.InsertRemoteDocumentApprovalParams{
			OrganizationID: orgID, DocumentVersionID: row.DocumentVersionID, ContentHash: hash, ApproverName: row.SignerName,
			CustomerID: row.CustomerID, DriverID: row.DriverID, ApprovedAt: ts(now), LinkID: pgtype.Int8{Int64: row.ID, Valid: true},
			OtpChallengeUuid: pgtype.UUID{Bytes: ver.ChallengeUUID, Valid: true}, VerifiedPhoneMasked: row.PhoneMasked,
			VerifiedPhoneHash: row.PhoneHash, IpAddress: clientIP(client.IP), UserAgent: ua,
		})
		if err != nil {
			return err
		}
		if _, err := q.ApproveDocumentApprovalLink(octx, db.ApproveDocumentApprovalLinkParams{OrganizationID: orgID, ID: row.ID, ApprovedAt: ts(now)}); err != nil {
			return err
		}
		if _, err := q.SetDocumentStatus(octx, db.SetDocumentStatusParams{OrganizationID: orgID, ID: row.DocumentID, Status: model.DocumentApproved}); err != nil {
			return err
		}
		data := map[string]any{
			"document_uuid": row.DocumentUuid, "kind": row.Kind, "version": row.VersionNumber, "content_hash": hash,
			"approval_uuid": approvalUUID, "approver": row.SignerName, "method": model.MethodRemoteSMS, "party": model.PartyCustomer,
			"signer_role": row.SignerRole, "phone": row.PhoneMasked,
		}
		if err := q.InsertReservationEvent(octx, db.InsertReservationEventParams{
			OrganizationID: orgID, ReservationID: row.ReservationID, Action: action, Data: mustJSON(data),
		}); err != nil {
			return err
		}
		if row.RentalID.Valid {
			return q.InsertRentalEvent(octx, db.InsertRentalEventParams{
				OrganizationID: orgID, RentalID: row.RentalID.Int64, Action: rentalmodel.EventDocumentApproved, Data: mustJSON(data),
			})
		}
		return nil
	})
	if err != nil {
		return model.PublicDocument{}, err
	}
	r.audit(octx, orgID, "documents.remote_approved", row.DocumentUuid, map[string]any{
		"link_uuid": row.Uuid, "version_uuid": row.VersionUuid, "version": row.VersionNumber, "content_hash": hash,
		"approval_uuid": approvalUUID, "signer_role": row.SignerRole, "approver": row.SignerName, "phone": row.PhoneMasked,
		"challenge_uuid": ver.ChallengeUUID, "ip": client.IP, "user_agent": ua,
	}, &client)
	s.requestPDF(octx, orgID, row.DocumentVersionID)
	s.publish(octx, model.ResourceDocuments, "updated", row.DocumentUuid)
	if full, err := s.q.GetDocument(octx, db.GetDocumentParams{OrganizationID: orgID, Uuid: row.DocumentUuid}); err == nil && full.RentalUuid.Valid {
		s.publish(octx, rentalmodel.ResourceRentals, "updated", full.RentalUuid.Bytes)
	}
	return s.PublicDocument(ctx, token)
}

// OpenPublicPDF streams the approval PDF of the link's own approval.
func (s *Service) OpenPublicPDF(ctx context.Context, token string) (PDFFile, error) {
	octx, orgID, row, err := s.resolveLink(ctx, token)
	if err != nil {
		return PDFFile{}, err
	}
	if row.Status != model.LinkApproved {
		return PDFFile{}, &LinkStateError{State: publicState(row, s.clock())}
	}
	a, err := s.q.GetDocumentApprovalForPDF(octx, db.GetDocumentApprovalForPDFParams{
		OrganizationID: orgID, DocumentID: row.DocumentID, LinkID: pgtype.Int8{Int64: row.ID, Valid: true},
	})
	if notFound(err) {
		return PDFFile{}, ErrLinkNotFound
	}
	if err != nil {
		return PDFFile{}, err
	}
	return s.approvalPDF(octx, orgID, row.DocumentUuid, row.Kind, row.ReservationReference, a)
}

// OpenApprovalPDF streams the approval PDF of one approval (staff,
// tenant.document.download).
func (s *Service) OpenApprovalPDF(ctx context.Context, orgID int64, docID, approvalID uuid.UUID) (PDFFile, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return PDFFile{}, err
	}
	a, err := s.q.GetDocumentApprovalForPDF(ctx, db.GetDocumentApprovalForPDFParams{
		OrganizationID: orgID, DocumentID: doc.ID, Uuid: pgtype.UUID{Bytes: approvalID, Valid: true},
	})
	if notFound(err) {
		return PDFFile{}, ErrApprovalNotFound
	}
	if err != nil {
		return PDFFile{}, err
	}
	return s.approvalPDF(ctx, orgID, doc.Uuid, doc.Kind, doc.ReservationReference, a)
}

// ErrApprovalNotFound: unknown approval (or another document's).
var ErrApprovalNotFound = errors.New("approval not found")
