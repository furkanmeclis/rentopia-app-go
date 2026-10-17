// Package usecase implements the WhatsApp channel (TEC-735,
// docs/design/whatsapp.md): one provider session per organization (wuzapi
// user) behind a WhatsApp integration account, QR linking, the session
// state with a drop alert, rate-limited transactional sends, the message
// log with delivery / read receipts, and the notification engine channel
// (Channel).
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits.
const (
	// PerRecipientPerHour caps messages to one number per hour.
	PerRecipientPerHour = 10
	// ReceiptTimeout: a sent message without a delivery receipt after
	// this long counts as not delivered (the engine runs the next
	// channel).
	ReceiptTimeout = 60 * time.Minute
	// WebhookPath is the public webhook route prefix (+ token).
	WebhookPath = "/v1/public/whatsapp/webhook/"
)

// Activity actions.
const (
	ActionConnectStarted = "whatsapp.connect_started"
	ActionConnected      = "whatsapp.connected"
	ActionDisconnected   = "whatsapp.disconnected"
	ActionLoggedOut      = "whatsapp.logged_out"
	ActionRemoved        = "whatsapp.removed"
	ActionSettings       = "whatsapp.settings_updated"
	ActionTestSent       = "whatsapp.test_sent"
)

var (
	// ErrNotConnected: no linked WhatsApp session.
	ErrNotConnected = errors.New("WhatsApp is not connected")
	// ErrNoAccount: no active WhatsApp account (nothing to act on).
	ErrNoAccount = errors.New("WhatsApp is not set up for this organization")
	// ErrProviderUnavailable: the provider is not registered / configured.
	ErrProviderUnavailable = errors.New("the WhatsApp gateway is not available on this server")
	// ErrRateLimited: the session or recipient limit is reached.
	ErrRateLimited = errors.New("WhatsApp send limit reached; try again later")
	// ErrNotOnWhatsApp: the number has no WhatsApp account.
	ErrNotOnWhatsApp = errors.New("the number is not on WhatsApp")
	// ErrInvalidPhone: no valid mobile number.
	ErrInvalidPhone = errors.New("no valid mobile number")
	// ErrWebhookUnknown: no session for the webhook token.
	ErrWebhookUnknown = errors.New("unknown webhook")
)

// Detail is a field-level validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError carries field details (400).
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

// ProviderError is a provider failure the caller should see (502).
type ProviderError struct{ Message string }

func (e *ProviderError) Error() string { return e.Message }

// Notifier is the notification center (drop alerts).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Config holds the environment settings.
type Config struct {
	// WebhookBaseURL is the API origin as the provider reaches it
	// ("http://backend.<net>:8080"); empty: no webhook (receipts then
	// time out).
	WebhookBaseURL string
}

// Service implements the WhatsApp channel.
type Service struct {
	q         *db.Queries
	accounts  *accountsusecase.Service
	vault     *integrations.Vault
	providers map[string]whatsapp.Provider
	order     []string
	cfg       Config
	activity  *activity.Recorder
	notifier  Notifier
	reporter  channel.Reporter
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service.
func New(q *db.Queries, accounts *accountsusecase.Service, vault *integrations.Vault, cfg Config, providers ...whatsapp.Provider) *Service {
	s := &Service{
		q: q, accounts: accounts, vault: vault, providers: map[string]whatsapp.Provider{}, cfg: cfg,
		log: slog.Default(), now: time.Now,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		s.providers[p.Provider()] = p
		s.order = append(s.order, p.Provider())
	}
	return s
}

// SetActivity wires the activity log.
func (s *Service) SetActivity(rec *activity.Recorder) { s.activity = rec }

// SetNotifier wires the notification center (drop alerts).
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetLogger sets the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock overrides the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func sha(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// newMessageID returns a WhatsApp style message id ("3EB0" + 32 hex).
func newMessageID() string { return "3EB0" + strings.ToUpper(randomHex(16)) }

// state is the organization's WhatsApp setup.
type state struct {
	acc  integrations.Account
	prov whatsapp.Provider
	sess *db.WhatsappSession
}

// current loads the active account, its provider and session.
func (s *Service) current(ctx context.Context, orgID int64) (state, error) {
	acc, err := s.accounts.ActiveAccount(ctx, orgID, whatsapp.Kind)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return state{}, ErrNoAccount
	}
	if err != nil {
		return state{}, err
	}
	return s.stateOf(ctx, orgID, acc)
}

func (s *Service) stateOf(ctx context.Context, orgID int64, acc integrations.Account) (state, error) {
	st := state{acc: acc, prov: s.providers[acc.Provider]}
	row, err := s.q.GetWhatsAppSessionByAccount(ctx, db.GetWhatsAppSessionByAccountParams{
		OrganizationID: orgID, IntegrationAccountID: acc.ID,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return st, err
	default:
		st.sess = &row
	}
	return st, nil
}

func (s *Service) providerInfos() []model.ProviderInfo {
	out := make([]model.ProviderInfo, 0, len(s.order))
	for _, code := range s.order {
		out = append(out, model.ProviderInfo{Code: code, Configured: s.providers[code].Configured()})
	}
	return out
}

func (s *Service) view(st state) model.Session {
	out := model.Session{
		Status: model.StatusNotConnected, RateLimitPerMinute: model.DefaultRateLimit, Providers: s.providerInfos(),
	}
	if st.acc.ID == 0 {
		return out
	}
	id := st.acc.UUID
	out.AccountUUID, out.Provider = &id, st.acc.Provider
	if st.sess == nil || whatsapp.Token(st.acc) == "" {
		return out
	}
	r := st.sess
	out.Status, out.PhoneMasked, out.LastError = r.Status, r.PhoneMasked, r.LastError
	out.ConnectedAt, out.DisconnectedAt = timePtr(r.ConnectedAt), timePtr(r.DisconnectedAt)
	out.LastStatusAt, out.RiskAcknowledgedAt = timePtr(r.LastStatusAt), timePtr(r.RiskAcknowledgedAt)
	out.RateLimitPerMinute = r.RateLimitPerMinute
	return out
}

func (s *Service) audit(ctx context.Context, orgID int64, action string, sess *db.WhatsappSession, payload map[string]any) {
	if s.activity == nil {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	var actor *int64
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		id := p.UserInternal
		actor = &id
	}
	var id *uuid.UUID
	if sess != nil {
		u := sess.Uuid
		id = &u
	}
	s.activity.Record(ctx, actor, action, model.Resource, id, payload, nil)
}

// Session returns the organization's WhatsApp state.
func (s *Service) Session(ctx context.Context, orgID int64) (model.Session, error) {
	st, err := s.current(ctx, orgID)
	if errors.Is(err, ErrNoAccount) {
		return s.view(state{}), nil
	}
	if err != nil {
		return model.Session{}, err
	}
	return s.view(st), nil
}

func (s *Service) defaultProvider() string {
	for _, code := range s.order {
		if s.providers[code].Configured() {
			return code
		}
	}
	return ""
}

func (s *Service) webhookURL(token string) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.WebhookBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + WebhookPath + token
}

// Connect links WhatsApp: it creates the WhatsApp integration account and
// the provider session when missing, records the risk notice and opens the
// socket; the QR code (QR) links the phone.
func (s *Service) Connect(ctx context.Context, orgID int64, in model.ConnectInput) (model.Session, error) {
	if !in.AcknowledgeRisk {
		return model.Session{}, &ValidationError{Details: []Detail{{
			Field: "acknowledge_risk", Code: "required", Message: "the number risk notice must be acknowledged",
		}}}
	}
	acc, err := s.accounts.ActiveAccount(ctx, orgID, whatsapp.Kind)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		code := strings.TrimSpace(in.Provider)
		if code == "" {
			code = s.defaultProvider()
		}
		prov, ok := s.providers[code]
		if !ok {
			return model.Session{}, &ValidationError{Details: []Detail{{Field: "provider", Code: "invalid", Message: "unknown WhatsApp provider"}}}
		}
		if !prov.Configured() {
			return model.Session{}, ErrProviderUnavailable
		}
		if _, err := s.accounts.Connect(ctx, orgID, accountsusecase.ConnectInput{
			Kind: whatsapp.Kind, Provider: code, DisplayName: "WhatsApp", Mode: integrations.ModeLive,
		}); err != nil && !errors.Is(err, accountsusecase.ErrActiveExists) {
			return model.Session{}, err
		}
		acc, err = s.accounts.ActiveAccount(ctx, orgID, whatsapp.Kind)
	}
	if err != nil {
		return model.Session{}, err
	}
	st, err := s.stateOf(ctx, orgID, acc)
	if err != nil {
		return model.Session{}, err
	}
	if st.prov == nil || !st.prov.Configured() {
		return model.Session{}, ErrProviderUnavailable
	}
	now := s.clock()
	if whatsapp.Token(st.acc) == "" || st.sess == nil {
		if st, err = s.createSession(ctx, orgID, st); err != nil {
			return model.Session{}, err
		}
	} else if err := s.q.AcknowledgeWhatsAppRisk(ctx, db.AcknowledgeWhatsAppRiskParams{
		RiskAcknowledgedAt: ts(now), RiskAcknowledgedBy: actorID(ctx), OrganizationID: orgID, ID: st.sess.ID,
	}); err != nil {
		return model.Session{}, err
	}
	err = st.prov.Connect(ctx, st.acc)
	var rej *whatsapp.Rejected
	if errors.As(err, &rej) && rej.Status == 401 {
		// The gateway lost the user (its database was reset): start over.
		if st, err = s.createSession(ctx, orgID, st); err == nil {
			err = st.prov.Connect(ctx, st.acc)
		}
	}
	if err != nil {
		return model.Session{}, providerErr(err)
	}
	sess, err := s.sync(ctx, orgID, st, "")
	if err != nil {
		return model.Session{}, err
	}
	if sess.Status == model.StatusCreated || sess.Status == model.StatusLoggedOut || sess.Status == model.StatusDisconnected {
		if sess, err = s.setStatus(ctx, orgID, sess, model.StatusPairing); err != nil {
			return model.Session{}, err
		}
	}
	st.sess = &sess
	s.audit(ctx, orgID, ActionConnectStarted, st.sess, map[string]any{"provider": st.acc.Provider})
	return s.view(st), nil
}

func providerErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, whatsapp.ErrNotConfigured) {
		return ErrProviderUnavailable
	}
	return &ProviderError{Message: truncate(err.Error(), 300)}
}

// createSession creates a provider session for the account (replacing a
// previous one) and seals its token into the account credentials.
func (s *Service) createSession(ctx context.Context, orgID int64, st state) (state, error) {
	if whatsapp.Token(st.acc) != "" {
		if err := st.prov.DeleteSession(ctx, st.acc); err != nil {
			s.log.Warn("whatsapp_delete_session_failed", "organization_id", orgID, "error", err)
		}
	}
	token, hook := randomHex(32), randomHex(32)
	created, err := st.prov.CreateSession(ctx, whatsapp.SessionInput{
		Name: "rentopia-" + st.acc.UUID.String(), Token: token, WebhookURL: s.webhookURL(hook),
	})
	if err != nil {
		return st, providerErr(err)
	}
	creds := integrations.Credentials{
		whatsapp.CredentialToken: token, whatsapp.CredentialUserID: created.UserID, whatsapp.CredentialWebhookToken: hook,
	}
	sealed, err := s.vault.Seal(orgID, st.acc.UUID, creds)
	if err != nil {
		return st, err
	}
	if _, err := s.q.SetWhatsAppAccountCredentials(ctx, db.SetWhatsAppAccountCredentialsParams{
		CredentialsCiphertext: sealed, CredentialsHint: []byte("{}"), UpdatedBy: actorID(ctx), OrganizationID: orgID, ID: st.acc.ID,
	}); err != nil {
		return st, err
	}
	now := s.clock()
	var row db.WhatsappSession
	if st.sess == nil {
		row, err = s.q.InsertWhatsAppSession(ctx, db.InsertWhatsAppSessionParams{
			OrganizationID: orgID, IntegrationAccountID: st.acc.ID, Provider: st.acc.Provider, WebhookTokenHash: sha(hook),
			RiskAcknowledgedAt: ts(now), RiskAcknowledgedBy: actorID(ctx),
		})
	} else {
		row, err = s.q.ResetWhatsAppSession(ctx, db.ResetWhatsAppSessionParams{
			Provider: st.acc.Provider, WebhookTokenHash: sha(hook), RiskAcknowledgedAt: ts(now),
			RiskAcknowledgedBy: actorID(ctx), OrganizationID: orgID, ID: st.sess.ID,
		})
	}
	if err != nil {
		return st, err
	}
	st.acc.Credentials = creds
	st.sess = &row
	return st, nil
}

func stateParams(r db.WhatsappSession, now time.Time) db.SetWhatsAppSessionStateParams {
	return db.SetWhatsAppSessionStateParams{
		Status: r.Status, PhoneMasked: r.PhoneMasked, ConnectedAt: r.ConnectedAt, DisconnectedAt: r.DisconnectedAt,
		LastStatusAt: ts(now), LastError: r.LastError, AlertedAt: r.AlertedAt, OrganizationID: r.OrganizationID, ID: r.ID,
	}
}

func (s *Service) setStatus(ctx context.Context, orgID int64, r db.WhatsappSession, status string) (db.WhatsappSession, error) {
	p := stateParams(r, s.clock())
	p.Status, p.OrganizationID = status, orgID
	return s.q.SetWhatsAppSessionState(ctx, p)
}

// sync reads the provider state and records it. event is a webhook event
// that decides what the state alone cannot (logged out). A drop of a
// linked session alerts the integration managers once.
func (s *Service) sync(ctx context.Context, orgID int64, st state, event string) (db.WhatsappSession, error) {
	if st.sess == nil {
		return db.WhatsappSession{}, ErrNotConnected
	}
	prev := *st.sess
	now := s.clock()
	p := stateParams(prev, now)
	p.OrganizationID = orgID
	status, err := st.prov.Status(ctx, st.acc)
	switch {
	case err != nil:
		p.LastError = truncate(err.Error(), 500)
		if prev.Status == model.StatusConnected {
			p.Status, p.DisconnectedAt = model.StatusDisconnected, ts(now)
		}
	case status.LoggedIn:
		p.Status, p.LastError = model.StatusConnected, ""
		if prev.Status != model.StatusConnected {
			p.ConnectedAt, p.DisconnectedAt, p.AlertedAt = ts(now), pgtype.Timestamptz{}, pgtype.Timestamptz{}
		}
		if phone := status.Phone(); phone != "" {
			p.PhoneMasked = pii.MaskPhone(phone)
		}
	default:
		p.LastError = ""
		switch prev.Status {
		case model.StatusConnected:
			p.Status, p.DisconnectedAt = model.StatusDisconnected, ts(now)
		case model.StatusCreated:
			if status.Connected {
				p.Status = model.StatusPairing
			}
		}
	}
	if event == whatsapp.EventLoggedOut && p.Status != model.StatusConnected {
		p.Status = model.StatusLoggedOut
		if !p.DisconnectedAt.Valid {
			p.DisconnectedAt = ts(now)
		}
	}
	dropped := prev.Status == model.StatusConnected &&
		(p.Status == model.StatusDisconnected || p.Status == model.StatusLoggedOut)
	alert := dropped && !prev.AlertedAt.Valid
	if alert {
		p.AlertedAt = ts(now)
	}
	row, err := s.q.SetWhatsAppSessionState(ctx, p)
	if err != nil {
		return prev, err
	}
	switch {
	case dropped:
		s.audit(ctx, orgID, ActionDisconnected, &row, map[string]any{"status": row.Status})
		if alert {
			s.alert(ctx, orgID)
		}
	case prev.Status != model.StatusConnected && row.Status == model.StatusConnected:
		s.audit(ctx, orgID, ActionConnected, &row, map[string]any{"phone": row.PhoneMasked})
	}
	return row, nil
}

// alert tells the members who manage integrations that WhatsApp dropped.
func (s *Service) alert(ctx context.Context, orgID int64) {
	if s.notifier == nil {
		return
	}
	rows, err := s.q.ListWhatsAppAlertRecipients(ctx, db.ListWhatsAppAlertRecipientsParams{
		OrganizationID: orgID, Permission: rbac.PermTenantIntegrationsManage,
	})
	if err != nil {
		s.log.Warn("whatsapp_alert_recipients_failed", "organization_id", orgID, "error", err)
		return
	}
	var action *string
	if org, err := s.q.GetOrganizationByID(ctx, orgID); err == nil {
		u := "/t/" + org.Slug + "/settings/whatsapp"
		action = &u
	}
	for _, r := range rows {
		title, body := "WhatsApp bağlantısı koptu",
			"WhatsApp oturumu bağlı değil; bildirimler SMS ile gönderilir. Yeniden bağlamak için WhatsApp ayarlarını açın."
		if r.Locale == "en" {
			title, body = "WhatsApp disconnected",
				"The WhatsApp session is not connected; notifications go out by SMS. Open the WhatsApp settings to reconnect."
		}
		org, uid := orgID, r.UserID
		if _, err := s.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			OrganizationID: &org, UserID: &uid, Channels: []string{notifmodel.ChannelInapp},
			Priority: notifmodel.PriorityHigh, Title: title, Body: body, SourceEvent: "whatsapp.disconnected",
			ActionURL: action, Language: r.Locale, Payload: map[string]any{"resource": model.Resource},
		}); err != nil {
			s.log.Warn("whatsapp_alert_failed", "organization_id", orgID, "error", err)
		}
	}
}

// QR returns the pairing QR code while the session waits for a phone.
func (s *Service) QR(ctx context.Context, orgID int64) (model.QR, error) {
	st, err := s.current(ctx, orgID)
	if err != nil {
		return model.QR{}, err
	}
	if st.sess == nil || whatsapp.Token(st.acc) == "" {
		return model.QR{}, ErrNotConnected
	}
	if st.prov == nil {
		return model.QR{}, ErrProviderUnavailable
	}
	sess, err := s.sync(ctx, orgID, st, "")
	if err != nil {
		return model.QR{}, err
	}
	st.sess = &sess
	if sess.Status == model.StatusConnected {
		return model.QR{Status: sess.Status}, nil
	}
	if err := st.prov.Connect(ctx, st.acc); err != nil {
		return model.QR{}, providerErr(err)
	}
	code, err := st.prov.QR(ctx, st.acc)
	if errors.Is(err, whatsapp.ErrAlreadyLoggedIn) {
		sess, err = s.sync(ctx, orgID, st, "")
		if err != nil {
			return model.QR{}, err
		}
		return model.QR{Status: sess.Status}, nil
	}
	if err != nil {
		return model.QR{}, providerErr(err)
	}
	if sess.Status != model.StatusPairing {
		if sess, err = s.setStatus(ctx, orgID, sess, model.StatusPairing); err != nil {
			return model.QR{}, err
		}
	}
	return model.QR{Status: sess.Status, QRCode: code}, nil
}

// Refresh reads the session state from the provider.
func (s *Service) Refresh(ctx context.Context, orgID int64) (model.Session, error) {
	st, err := s.current(ctx, orgID)
	if err != nil {
		return model.Session{}, err
	}
	if st.sess == nil || whatsapp.Token(st.acc) == "" || st.prov == nil {
		return s.view(st), nil
	}
	sess, err := s.sync(ctx, orgID, st, "")
	if err != nil {
		return model.Session{}, err
	}
	st.sess = &sess
	return s.view(st), nil
}

// Logout unlinks the phone; Connect + QR link one again.
func (s *Service) Logout(ctx context.Context, orgID int64) (model.Session, error) {
	st, err := s.current(ctx, orgID)
	if err != nil {
		return model.Session{}, err
	}
	if st.sess == nil || whatsapp.Token(st.acc) == "" {
		return model.Session{}, ErrNotConnected
	}
	if st.prov == nil {
		return model.Session{}, ErrProviderUnavailable
	}
	if err := st.prov.Logout(ctx, st.acc); err != nil {
		var rej *whatsapp.Rejected
		if !errors.As(err, &rej) {
			return model.Session{}, providerErr(err)
		}
		// Not logged in at the provider: record the state anyway.
	}
	now := s.clock()
	p := stateParams(*st.sess, now)
	p.OrganizationID, p.Status, p.PhoneMasked, p.LastError = orgID, model.StatusLoggedOut, "", ""
	p.DisconnectedAt = ts(now)
	row, err := s.q.SetWhatsAppSessionState(ctx, p)
	if err != nil {
		return model.Session{}, err
	}
	st.sess = &row
	s.audit(ctx, orgID, ActionLoggedOut, &row, nil)
	return s.view(st), nil
}

// Remove deletes the provider session and disables the WhatsApp account;
// Connect starts over with a new account.
func (s *Service) Remove(ctx context.Context, orgID int64) (model.Session, error) {
	st, err := s.current(ctx, orgID)
	if err != nil {
		return model.Session{}, err
	}
	if st.prov != nil && whatsapp.Token(st.acc) != "" {
		_ = st.prov.Logout(ctx, st.acc)
		if err := st.prov.DeleteSession(ctx, st.acc); err != nil {
			s.log.Warn("whatsapp_delete_session_failed", "organization_id", orgID, "error", err)
		}
	}
	if st.sess != nil {
		now := s.clock()
		p := stateParams(*st.sess, now)
		p.OrganizationID, p.Status, p.PhoneMasked = orgID, model.StatusLoggedOut, ""
		p.DisconnectedAt = ts(now)
		if row, err := s.q.SetWhatsAppSessionState(ctx, p); err == nil {
			st.sess = &row
		}
	}
	if _, err := s.accounts.SetStatus(ctx, orgID, st.acc.UUID, accountsusecase.StatusDisabled); err != nil {
		return model.Session{}, err
	}
	s.audit(ctx, orgID, ActionRemoved, st.sess, map[string]any{"provider": st.acc.Provider})
	return s.view(state{}), nil
}

// UpdateSettings changes the session settings (rate limit).
func (s *Service) UpdateSettings(ctx context.Context, orgID int64, in model.SettingsInput) (model.Session, error) {
	st, err := s.current(ctx, orgID)
	if err != nil {
		return model.Session{}, err
	}
	if st.sess == nil {
		return model.Session{}, ErrNotConnected
	}
	if in.RateLimitPerMinute != nil {
		v := *in.RateLimitPerMinute
		if v < model.MinRateLimit || v > model.MaxRateLimit {
			return model.Session{}, &ValidationError{Details: []Detail{{
				Field: "rate_limit_per_minute", Code: "out_of_range",
				Message: fmt.Sprintf("rate_limit_per_minute must be between %d and %d", model.MinRateLimit, model.MaxRateLimit),
			}}}
		}
		row, err := s.q.SetWhatsAppSessionSettings(ctx, db.SetWhatsAppSessionSettingsParams{
			RateLimitPerMinute: v, OrganizationID: orgID, ID: st.sess.ID,
		})
		if err != nil {
			return model.Session{}, err
		}
		if row.RateLimitPerMinute != st.sess.RateLimitPerMinute {
			s.audit(ctx, orgID, ActionSettings, &row, map[string]any{
				"rate_limit_per_minute": map[string]any{"old": st.sess.RateLimitPerMinute, "new": row.RateLimitPerMinute},
			})
		}
		st.sess = &row
	}
	return s.view(st), nil
}

// webhookSession resolves a webhook token.
func (s *Service) webhookSession(ctx context.Context, token string) (state, error) {
	token = strings.TrimSpace(token)
	if len(token) < 32 {
		return state{}, ErrWebhookUnknown
	}
	row, err := s.q.GetWhatsAppSessionByWebhook(ctx, sha(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return state{}, ErrWebhookUnknown
	}
	if err != nil {
		return state{}, err
	}
	acc, err := s.accounts.AccountByID(ctx, row.OrganizationID, row.IntegrationAccountID)
	if err != nil {
		return state{}, err
	}
	want := acc.Credentials[whatsapp.CredentialWebhookToken]
	if subtle.ConstantTimeCompare([]byte(want), []byte(token)) != 1 {
		return state{}, ErrWebhookUnknown
	}
	return state{acc: acc, prov: s.providers[acc.Provider], sess: &row}, nil
}

// HandleWebhook applies one provider webhook delivery (receipts, session
// state).
func (s *Service) HandleWebhook(ctx context.Context, token, contentType string, body []byte) error {
	st, err := s.webhookSession(ctx, token)
	if err != nil {
		return err
	}
	if st.prov == nil {
		return ErrProviderUnavailable
	}
	events, err := st.prov.ParseWebhook(contentType, body)
	if err != nil {
		return &ValidationError{Details: []Detail{{Field: "body", Code: "invalid", Message: err.Error()}}}
	}
	orgID := st.sess.OrganizationID
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	for _, ev := range events {
		switch ev.Type {
		case whatsapp.EventReceipt:
			for _, id := range ev.MessageIDs {
				s.ApplyReceipt(ctx, orgID, id, ev.Receipt, ev.At)
			}
		case whatsapp.EventConnected, whatsapp.EventPaired, whatsapp.EventDisconnected, whatsapp.EventLoggedOut:
			if whatsapp.Token(st.acc) == "" {
				continue
			}
			sess, err := s.sync(ctx, orgID, st, ev.Type)
			if err != nil {
				return err
			}
			st.sess = &sess
		}
	}
	return nil
}
