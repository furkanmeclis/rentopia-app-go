package usecase

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Code timings the sign-in page shows (the OTP service enforces them).
const (
	codeResendSeconds = 60
	codeTTLSeconds    = 300
)

var referenceRE = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{2,31}$`)

// Contact is what the customer typed, normalized: a phone number or an
// e-mail address.
type Contact struct {
	Channel    string
	Normalized string
}

// ParseContact classifies and normalizes a phone number or an e-mail
// address (400 when it is neither).
func ParseContact(raw string) (Contact, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "@") {
		e := pii.Email(raw)
		if !smsusecase.ValidEmail(e) {
			return Contact{}, validation("contact", "invalid", "enter the phone number or e-mail address on your reservation")
		}
		return Contact{Channel: model.ChannelEmail, Normalized: e}, nil
	}
	p := pii.Phone(raw)
	if !pii.ValidPhone(p) {
		return Contact{}, validation("contact", "invalid", "enter the phone number or e-mail address on your reservation")
	}
	return Contact{Channel: model.ChannelSMS, Normalized: p}, nil
}

// NormalizeReference upper-cases and trims a reservation reference.
func NormalizeReference(raw string) (string, error) {
	ref := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if !referenceRE.MatchString(ref) {
		return "", validation("reference", "invalid", "enter the reservation reference")
	}
	return ref, nil
}

// ContactKey is the rate-limit subject of a contact inside an organization
// (a keyed hash prefix, never the contact itself).
func (s *Service) ContactKey(orgID int64, c Contact) string {
	return s.contactHash(orgID, c)[:32]
}

func (s *Service) contactHash(orgID int64, c Contact) string {
	if c.Channel == model.ChannelEmail {
		return s.otp.EmailHash(orgID, c.Normalized)
	}
	return s.otp.PhoneHash(orgID, c.Normalized)
}

func (s *Service) channelAvailable(ctx context.Context, orgID int64, channel string) bool {
	if channel == model.ChannelEmail {
		return s.otp.EmailAvailable()
	}
	return s.otp.HasActiveAccount(ctx, orgID)
}

func subject(reservation uuid.UUID) string { return "reservation:" + reservation.String() }

// match is a reservation reference + contact that matched.
type match struct {
	reservationID   int64
	reservationUUID uuid.UUID
	role            string
	customerID      int64
	driverID        pgtype.Int8
}

// lookup finds the reservation by reference and compares the contact with
// the renter's and the drivers' through keyed hashes (constant time, every
// candidate compared). nil = no match (unknown reference or other
// contact; the caller answers the same either way). A renter match wins
// over a driver match.
func (s *Service) lookup(ctx context.Context, orgID int64, ref string, c Contact) (*match, error) {
	res, err := s.q.GetPortalReservationByReference(ctx, db.GetPortalReservationByReferenceParams{OrganizationID: orgID, Reference: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		// Spend a comparable amount of work on an unknown reference.
		_ = s.contactHash(orgID, c)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	contacts, err := s.q.ListPortalContacts(ctx, db.ListPortalContactsParams{OrganizationID: orgID, ReservationID: res.ID})
	if err != nil {
		return nil, err
	}
	want := []byte(s.contactHash(orgID, c))
	var found *match
	for _, row := range contacts {
		var value string
		if c.Channel == model.ChannelEmail {
			value = pii.Email(row.Email)
			if !smsusecase.ValidEmail(value) {
				value = ""
			}
		} else {
			value = pii.Phone(row.Phone)
			if !pii.ValidPhone(value) {
				value = ""
			}
		}
		got := []byte(s.contactHash(orgID, Contact{Channel: c.Channel, Normalized: value}))
		if value == "" || subtle.ConstantTimeCompare(got, want) != 1 {
			continue
		}
		m := &match{reservationID: res.ID, reservationUUID: res.Uuid, role: row.Role, customerID: row.CustomerID, driverID: row.DriverID}
		if found == nil || (found.role == model.RoleDriver && m.role == model.RoleRenter) {
			found = m
		}
	}
	if found != nil && found.role == model.RoleRenter {
		found.driverID = pgtype.Int8{}
	}
	return found, nil
}

// Start sends a sign-in code when the reference and the contact match. The
// answer is the same whether they matched or not (nothing tells whether
// the reference exists or which half was wrong); only an organization that
// cannot send codes on the chosen channel answers ErrChannelUnavailable,
// before anything is looked up.
func (s *Service) Start(ctx context.Context, slug string, in model.StartInput, client model.ClientInfo) (model.StartResult, error) {
	org, err := s.organization(ctx, slug)
	if err != nil {
		return model.StartResult{}, err
	}
	ref, refErr := NormalizeReference(in.Reference)
	contact, contactErr := ParseContact(in.Contact)
	if ve := joinValidation(refErr, contactErr); ve != nil {
		return model.StartResult{}, ve
	}
	if !s.channelAvailable(ctx, org.ID, contact.Channel) {
		return model.StartResult{}, ErrChannelUnavailable
	}
	octx := orgContext(ctx, org)
	out := model.StartResult{Status: "code_sent_if_matched", Channel: contact.Channel, ResendAfterSeconds: codeResendSeconds, CodeTTLSeconds: codeTTLSeconds}
	m, err := s.lookup(octx, org.ID, ref, contact)
	if err != nil {
		return model.StartResult{}, err
	}
	masked := maskContact(contact)
	if m == nil {
		s.audit(octx, "customer_portal.sign_in_unmatched", model.ResourcePortal, nil, map[string]any{
			"reference": ref, "channel": contact.Channel, "contact": masked, "step": "start",
		}, &client)
		return out, nil
	}
	var ch smsusecase.Challenge
	if contact.Channel == model.ChannelEmail {
		ch, err = s.otp.IssueEmailOTP(octx, org.ID, model.OTPPurpose, subject(m.reservationUUID), contact.Normalized, smsusecase.EmailOTP{Sender: org.Name})
	} else {
		ch, err = s.otp.IssueOTP(octx, org.ID, model.OTPPurpose, subject(m.reservationUUID), contact.Normalized)
	}
	if err != nil {
		// Generic on purpose: a cooldown, a limit or a provider failure
		// would otherwise tell that the details matched.
		s.log.Info("customer_portal_code_not_sent", "organization_id", org.ID, "reservation", m.reservationUUID, "error", err)
		s.audit(octx, "customer_portal.code_not_sent", model.ResourcePortal, &m.reservationUUID, map[string]any{
			"reference": ref, "channel": contact.Channel, "contact": masked, "role": m.role, "reason": codeErrorReason(err),
		}, &client)
		return out, nil
	}
	s.audit(octx, "customer_portal.code_sent", model.ResourcePortal, &m.reservationUUID, map[string]any{
		"reference": ref, "channel": contact.Channel, "contact": masked, "role": m.role, "challenge_uuid": ch.UUID,
	}, &client)
	return out, nil
}

func codeErrorReason(err error) string {
	var cd *smsusecase.CooldownError
	switch {
	case errors.As(err, &cd):
		return "cooldown"
	case errors.Is(err, smsusecase.ErrOTPRateLimited), errors.Is(err, smsusecase.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, smsusecase.ErrNoAccount), errors.Is(err, smsusecase.ErrNoEmailChannel):
		return "no_channel"
	}
	return "send_failed"
}

func maskContact(c Contact) string {
	if c.Channel == model.ChannelEmail {
		return pii.MaskEmail(c.Normalized)
	}
	return pii.MaskPhone(c.Normalized)
}

func joinValidation(errs ...error) error {
	out := &ValidationError{}
	for _, err := range errs {
		var ve *ValidationError
		if errors.As(err, &ve) {
			out.Details = append(out.Details, ve.Details...)
		}
	}
	if len(out.Details) == 0 {
		return nil
	}
	return out
}

// Verify checks the code and opens a customer session. Every failure is
// the same ErrCodeInvalid.
func (s *Service) Verify(ctx context.Context, slug string, in model.VerifyInput, client model.ClientInfo) (model.SessionIssued, error) {
	org, err := s.organization(ctx, slug)
	if err != nil {
		return model.SessionIssued{}, err
	}
	ref, refErr := NormalizeReference(in.Reference)
	contact, contactErr := ParseContact(in.Contact)
	if ve := joinValidation(refErr, contactErr); ve != nil {
		return model.SessionIssued{}, ve
	}
	octx := orgContext(ctx, org)
	masked := maskContact(contact)
	fail := func(reservation *uuid.UUID, reason string) error {
		s.audit(octx, "customer_portal.sign_in_failed", model.ResourcePortal, reservation, map[string]any{
			"reference": ref, "channel": contact.Channel, "contact": masked, "reason": reason,
		}, &client)
		return ErrCodeInvalid
	}
	m, err := s.lookup(octx, org.ID, ref, contact)
	if err != nil {
		return model.SessionIssued{}, err
	}
	if m == nil {
		return model.SessionIssued{}, fail(nil, "unmatched")
	}
	ver, err := s.otp.VerifyOTP(octx, org.ID, model.OTPPurpose, subject(m.reservationUUID), in.Code)
	if err != nil {
		var ve *smsusecase.ValidationError
		switch {
		case errors.Is(err, smsusecase.ErrOTPLocked):
			return model.SessionIssued{}, fail(&m.reservationUUID, "locked")
		case errors.Is(err, smsusecase.ErrOTPExpired):
			return model.SessionIssued{}, fail(&m.reservationUUID, "expired")
		case errors.Is(err, smsusecase.ErrOTPInvalid), errors.Is(err, smsusecase.ErrOTPNotFound), errors.As(err, &ve):
			return model.SessionIssued{}, fail(&m.reservationUUID, "invalid")
		}
		return model.SessionIssued{}, err
	}
	// The code must have gone to the contact typed now (the subject's open
	// code may have been requested with another contact of the same
	// reservation).
	bind, err := s.q.GetPortalOTPBinding(octx, db.GetPortalOTPBindingParams{OrganizationID: org.ID, Uuid: ver.ChallengeUUID})
	if err != nil || bind.Channel != contact.Channel || subtle.ConstantTimeCompare([]byte(bind.PhoneHash), []byte(s.contactHash(org.ID, contact))) != 1 {
		return model.SessionIssued{}, fail(&m.reservationUUID, "other_contact")
	}
	token, err := newToken()
	if err != nil {
		return model.SessionIssued{}, err
	}
	now := s.clock()
	hard := now.Add(s.limits.SessionMax)
	expires := now.Add(s.limits.SessionIdle)
	if expires.After(hard) {
		expires = hard
	}
	var ip *netip.Addr
	if a, err := netip.ParseAddr(strings.TrimSpace(client.IP)); err == nil {
		ip = &a
	}
	row, err := s.q.InsertPortalSession(octx, db.InsertPortalSessionParams{
		OrganizationID: org.ID, CustomerID: m.customerID, DriverID: m.driverID, ReservationID: m.reservationID,
		TokenHash: TokenHash(token), Channel: contact.Channel, ContactMasked: clip(masked, 64), OtpChallengeUuid: ver.ChallengeUUID,
		IpAddress: ip, UserAgent: clip(strings.TrimSpace(client.UserAgent), 512), Now: ts(now), ExpiresAt: ts(expires),
		HardExpiresAt: ts(hard),
	})
	if err != nil {
		return model.SessionIssued{}, err
	}
	s.audit(octx, "customer_portal.signed_in", model.ResourcePortal, &m.reservationUUID, map[string]any{
		"reference": ref, "channel": contact.Channel, "contact": masked, "role": m.role, "session_uuid": row.Uuid,
		"challenge_uuid": ver.ChallengeUUID, "hard_expires_at": hard.Format(time.RFC3339),
	}, &client)
	p, err := s.Authenticate(ctx, slug, token)
	if err != nil {
		return model.SessionIssued{}, err
	}
	return model.SessionIssued{SessionToken: token, Session: p.View()}, nil
}

// Principal is an authenticated customer session.
type Principal struct {
	SessionID     int64
	SessionUUID   uuid.UUID
	Org           db.GetPortalOrganizationRow
	CustomerID    int64
	DriverID      pgtype.Int8
	Role          string
	DisplayName   string
	Channel       string
	ContactMasked string
	ExpiresAt     time.Time
	HardExpiresAt time.Time
}

// View is the session as the portal shows it.
func (p Principal) View() model.Session {
	return model.Session{
		Organization: model.Organization{Slug: p.Org.Slug, Name: p.Org.Name}, Role: p.Role, DisplayName: p.DisplayName,
		Channel: p.Channel, ContactMasked: p.ContactMasked, ExpiresAt: p.ExpiresAt, HardExpiresAt: p.HardExpiresAt,
	}
}

var tokenRE = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// Authenticate resolves a bearer token for the organization in the URL:
// unknown, revoked, idle-expired or past the hard cap, or another
// organization's session → ErrSessionInvalid. A valid session slides its
// idle expiry (at most once per TouchEvery, never beyond the hard cap).
func (s *Service) Authenticate(ctx context.Context, slug, token string) (Principal, error) {
	token = strings.TrimSpace(token)
	if !tokenRE.MatchString(token) {
		return Principal{}, ErrSessionInvalid
	}
	row, err := s.q.GetPortalSessionByToken(ctx, TokenHash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrSessionInvalid
	}
	if err != nil {
		return Principal{}, err
	}
	if !strings.EqualFold(row.OrganizationSlug, strings.TrimSpace(slug)) {
		return Principal{}, ErrSessionInvalid
	}
	org, err := s.organization(ctx, slug)
	if err != nil || org.ID != row.OrganizationID {
		return Principal{}, ErrSessionInvalid
	}
	now := s.clock()
	if row.RevokedAt.Valid || !now.Before(row.ExpiresAt.Time) || !now.Before(row.HardExpiresAt.Time) {
		return Principal{}, ErrSessionInvalid
	}
	expires := row.ExpiresAt.Time.UTC()
	if now.Sub(row.LastSeenAt.Time) >= s.limits.TouchEvery {
		next := now.Add(s.limits.SessionIdle)
		if next.After(row.HardExpiresAt.Time) {
			next = row.HardExpiresAt.Time.UTC()
		}
		if _, err := s.q.TouchPortalSession(ctx, db.TouchPortalSessionParams{
			OrganizationID: org.ID, ID: row.ID, Now: ts(now), ExpiresAt: ts(next),
		}); err == nil {
			expires = next
		}
	}
	p := Principal{
		SessionID: row.ID, SessionUUID: row.Uuid, Org: org, CustomerID: row.CustomerID, DriverID: row.DriverID,
		Role: model.RoleRenter, DisplayName: displayName(row.CustomerType, row.FirstName, row.LastName, row.CompanyTitle),
		Channel: row.Channel, ContactMasked: row.ContactMasked, ExpiresAt: expires, HardExpiresAt: row.HardExpiresAt.Time.UTC(),
	}
	if row.DriverID.Valid {
		p.Role = model.RoleDriver
		p.DisplayName = shortName(strings.TrimSpace(row.DriverFirstName.String + " " + row.DriverLastName.String))
	}
	return p, nil
}

// Logout revokes the session.
func (s *Service) Logout(ctx context.Context, p Principal, client model.ClientInfo) error {
	if _, err := s.q.RevokePortalSession(ctx, db.RevokePortalSessionParams{
		OrganizationID: p.Org.ID, ID: p.SessionID, Now: ts(s.clock()), Reason: "logout",
	}); err != nil {
		return err
	}
	s.audit(orgContext(ctx, p.Org), "customer_portal.signed_out", model.ResourcePortal, nil, map[string]any{
		"session_uuid": p.SessionUUID, "role": p.Role,
	}, &client)
	return nil
}

// ---------------------------------------------------------------- context

type principalKey struct{}

// WithPrincipal stores the session on the context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the session of a request behind the session
// middleware.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
