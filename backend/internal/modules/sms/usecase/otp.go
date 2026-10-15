package usecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// OTP challenge statuses.
const (
	OTPPending    = "pending"
	OTPVerified   = "verified"
	OTPExpired    = "expired"
	OTPLocked     = "locked"
	OTPSuperseded = "superseded"
)

// OTPCodeLength is the number of digits of a code.
const OTPCodeLength = 6

var (
	// ErrOTPNotFound: no open challenge for the purpose and subject.
	ErrOTPNotFound = errors.New("no open verification code; request a new one")
	// ErrOTPExpired: the code's lifetime is over.
	ErrOTPExpired = errors.New("the verification code expired; request a new one")
	// ErrOTPLocked: too many wrong codes.
	ErrOTPLocked = errors.New("too many wrong codes; request a new one")
	// ErrOTPInvalid: the code does not match.
	ErrOTPInvalid = errors.New("the verification code is wrong")
	// ErrOTPRateLimited: too many codes for the number or organization.
	ErrOTPRateLimited = errors.New("too many verification codes requested; try again later")
)

// CooldownError: a code was sent for the subject a moment ago.
type CooldownError struct{ RetryAfter time.Duration }

func (e *CooldownError) Error() string {
	return fmt.Sprintf("a code was just sent; try again in %d seconds", int(e.RetryAfter.Seconds()+0.5))
}

// Challenge is an issued code (never the code itself).
type Challenge struct {
	UUID        uuid.UUID `json:"uuid"`
	Purpose     string    `json:"purpose"`
	PhoneMasked string    `json:"phone_masked"`
	ExpiresAt   time.Time `json:"expires_at"`
	ResendAfter time.Time `json:"resend_after"`
	MaxAttempts int32     `json:"max_attempts"`
	Message     Sent      `json:"message"`
}

// Verification is the result of a correct code.
type Verification struct {
	ChallengeUUID uuid.UUID `json:"challenge_uuid"`
	VerifiedAt    time.Time `json:"verified_at"`
}

// InvalidCodeError is a wrong code with the attempts left.
type InvalidCodeError struct{ AttemptsLeft int32 }

func (e *InvalidCodeError) Error() string { return ErrOTPInvalid.Error() }

// Unwrap lets errors.Is(err, ErrOTPInvalid) match.
func (e *InvalidCodeError) Unwrap() error { return ErrOTPInvalid }

func codeScope(orgID int64, challenge uuid.UUID) string {
	return orgScope(orgID) + "|otp:" + challenge.String()
}

// codeHash is HMAC-SHA256 under the "sms.otp.code" key (HKDF from the
// master key), bound to the organization and the challenge.
func (s *Service) codeHash(orgID int64, challenge uuid.UUID, code string) string {
	return s.cipher.BlindIndex(PurposeOTPCode, codeScope(orgID, challenge), code)
}

// newCode draws a uniform numeric code from crypto/rand.
func newCode() (string, error) {
	max := big.NewInt(1)
	for range OTPCodeLength {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", OTPCodeLength, n), nil
}

func otpBody(code string, ttl time.Duration) string {
	return fmt.Sprintf("Doğrulama kodunuz: %s. Kod %d dakika geçerlidir; kimseyle paylaşmayın.", code, int(ttl.Minutes()+0.5))
}

func validSubject(purpose, subject string) error {
	if !purposeRE.MatchString(purpose) {
		return &ValidationError{Field: "purpose", Code: "invalid", Message: "purpose must be a lower-case code"}
	}
	if strings.TrimSpace(subject) == "" || len(subject) > 160 {
		return &ValidationError{Field: "subject_ref", Code: "invalid", Message: "subject_ref is required (at most 160 characters)"}
	}
	return nil
}

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IssueOTP creates a one-time code for (purpose, subjectRef), sends it by
// SMS (class otp, critical queue) and returns the challenge. A new code
// supersedes the subject's open one. Limits: a resend cooldown per subject,
// codes per number and per organization per hour (DB counters). The code
// is stored only as an HMAC; the SMS log keeps the text redacted.
func (s *Service) IssueOTP(ctx context.Context, orgID int64, purpose, subjectRef, phone string) (Challenge, error) {
	if err := validSubject(purpose, subjectRef); err != nil {
		return Challenge{}, err
	}
	normalized, err := NormalizePhone(phone)
	if err != nil {
		return Challenge{}, err
	}
	now := s.now()
	last, err := s.q.LatestOTPChallenge(ctx, db.LatestOTPChallengeParams{OrganizationID: orgID, Purpose: purpose, SubjectRef: subjectRef})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Challenge{}, err
	}
	if err == nil && s.limits.OTPCooldown > 0 {
		if wait := last.CreatedAt.Time.Add(s.limits.OTPCooldown).Sub(now); wait > 0 {
			return Challenge{}, &CooldownError{RetryAfter: wait}
		}
	}
	phoneHash := s.PhoneHash(orgID, normalized)
	since := pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}
	if s.limits.OTPPerPhoneHour > 0 {
		n, err := s.q.CountOTPChallengesSince(ctx, db.CountOTPChallengesSinceParams{
			OrganizationID: orgID, PhoneHash: pgtype.Text{String: phoneHash, Valid: true}, Since: since,
		})
		if err != nil {
			return Challenge{}, err
		}
		if n >= s.limits.OTPPerPhoneHour {
			s.audit(ctx, orgID, "sms.otp.rate_limited", ResourceOTP, uuid.Nil, map[string]any{
				"purpose": purpose, "subject_ref": subjectRef, "phone": pii.MaskPhone(normalized), "limit": "phone",
			})
			return Challenge{}, ErrOTPRateLimited
		}
	}
	if s.limits.OTPPerOrgHour > 0 {
		n, err := s.q.CountOTPChallengesSince(ctx, db.CountOTPChallengesSinceParams{OrganizationID: orgID, Since: since})
		if err != nil {
			return Challenge{}, err
		}
		if n >= s.limits.OTPPerOrgHour {
			return Challenge{}, ErrOTPRateLimited
		}
	}
	code, err := newCode()
	if err != nil {
		return Challenge{}, err
	}
	id := uuid.New()
	expires := now.Add(s.limits.OTPTTL)
	var challengeID int64
	err = s.tx(ctx, func(q *db.Queries) error {
		if err := q.SupersedeOpenOTPChallenges(ctx, db.SupersedeOpenOTPChallengesParams{
			OrganizationID: orgID, Purpose: purpose, SubjectRef: subjectRef,
		}); err != nil {
			return err
		}
		var err error
		challengeID, err = q.InsertOTPChallenge(ctx, db.InsertOTPChallengeParams{
			Uuid: id, OrganizationID: orgID, Purpose: purpose, SubjectRef: subjectRef, PhoneMasked: pii.MaskPhone(normalized),
			PhoneHash: phoneHash, CodeHash: s.codeHash(orgID, id, code), MaxAttempts: s.limits.OTPMaxAttempts,
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		return err
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		// A concurrent request issued a code for the same subject.
		return Challenge{}, &CooldownError{RetryAfter: s.limits.OTPCooldown}
	}
	if err != nil {
		return Challenge{}, err
	}
	sent, err := s.Send(ctx, orgID, SendInput{
		Class: sms.ClassOTP, Purpose: purpose, Phone: normalized, Body: otpBody(code, s.limits.OTPTTL),
		RedactedBody: otpBody(strings.Repeat("•", OTPCodeLength), s.limits.OTPTTL), RefType: "sms_otp", RefUUID: &id,
	})
	code = ""
	if err != nil {
		// No usable code without its SMS: close the challenge.
		_ = s.q.ExpireOTPChallenge(ctx, db.ExpireOTPChallengeParams{OrganizationID: orgID, ID: challengeID})
		s.audit(ctx, orgID, "sms.otp.send_failed", ResourceOTP, id, map[string]any{
			"purpose": purpose, "subject_ref": subjectRef, "phone": pii.MaskPhone(normalized), "error": err.Error(),
		})
		return Challenge{}, err
	}
	if msg, err := s.q.GetSMSMessageForSend(ctx, db.GetSMSMessageForSendParams{OrganizationID: orgID, Uuid: sent.MessageUUID}); err == nil {
		_ = s.q.SetOTPChallengeMessage(ctx, db.SetOTPChallengeMessageParams{
			OrganizationID: orgID, ID: challengeID, SmsMessageID: pgtype.Int8{Int64: msg.ID, Valid: true},
		})
	}
	s.audit(ctx, orgID, "sms.otp.issued", ResourceOTP, id, map[string]any{
		"purpose": purpose, "subject_ref": subjectRef, "phone": pii.MaskPhone(normalized),
		"expires_at": expires.UTC().Format(time.RFC3339), "sms_message_uuid": sent.MessageUUID,
	})
	return Challenge{
		UUID: id, Purpose: purpose, PhoneMasked: pii.MaskPhone(normalized), ExpiresAt: expires.UTC(),
		ResendAfter: now.Add(s.limits.OTPCooldown).UTC(), MaxAttempts: s.limits.OTPMaxAttempts, Message: sent,
	}, nil
}

// VerifyOTP checks a code against the subject's open challenge in constant
// time. A correct code verifies the challenge once; a wrong one counts an
// attempt and locks the challenge at the limit; an expired one closes it.
func (s *Service) VerifyOTP(ctx context.Context, orgID int64, purpose, subjectRef, code string) (Verification, error) {
	if err := validSubject(purpose, subjectRef); err != nil {
		return Verification{}, err
	}
	code = pii.Digits(code)
	var (
		out       Verification
		resultErr error
		action    string
		challenge uuid.UUID
		left      int32
		message   pgtype.Int8
	)
	err := s.tx(ctx, func(q *db.Queries) error {
		c, err := q.GetOpenOTPChallengeForUpdate(ctx, db.GetOpenOTPChallengeForUpdateParams{
			OrganizationID: orgID, Purpose: purpose, SubjectRef: subjectRef,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			resultErr, action = ErrOTPNotFound, "sms.otp.verify_failed"
			return nil
		}
		if err != nil {
			return err
		}
		challenge, message = c.Uuid, c.SmsMessageID
		now := s.now()
		if !now.Before(c.ExpiresAt.Time) {
			resultErr, action = ErrOTPExpired, "sms.otp.expired"
			return q.ExpireOTPChallenge(ctx, db.ExpireOTPChallengeParams{OrganizationID: orgID, ID: c.ID})
		}
		// Hash whatever was entered (also a malformed one) so the time does
		// not depend on the input; compare in constant time.
		got := s.codeHash(orgID, c.Uuid, code)
		match := subtle.ConstantTimeCompare([]byte(got), []byte(c.CodeHash)) == 1 && len(code) == OTPCodeLength
		if match {
			if err := q.RecordOTPAttempt(ctx, db.RecordOTPAttemptParams{OrganizationID: orgID, ID: c.ID, Status: OTPVerified}); err != nil {
				return err
			}
			out = Verification{ChallengeUUID: c.Uuid, VerifiedAt: now.UTC()}
			action = "sms.otp.verified"
			return nil
		}
		attempts := c.Attempts + 1
		status := OTPPending
		if attempts >= c.MaxAttempts {
			status = OTPLocked
		}
		if err := q.RecordOTPAttempt(ctx, db.RecordOTPAttemptParams{OrganizationID: orgID, ID: c.ID, Status: status}); err != nil {
			return err
		}
		left = c.MaxAttempts - attempts
		if status == OTPLocked {
			resultErr, action = ErrOTPLocked, "sms.otp.locked"
		} else {
			resultErr, action = &InvalidCodeError{AttemptsLeft: left}, "sms.otp.verify_failed"
		}
		return nil
	})
	if err != nil {
		return Verification{}, err
	}
	if message.Valid && (resultErr == nil || errors.Is(resultErr, ErrOTPLocked) || errors.Is(resultErr, ErrOTPExpired)) {
		// The code is no longer usable: drop the sealed SMS text now (a
		// resend of this message would deliver a dead code anyway).
		_ = s.q.ClearSMSPayload(ctx, db.ClearSMSPayloadParams{OrganizationID: orgID, ID: message.Int64})
	}
	payload := map[string]any{"purpose": purpose, "subject_ref": subjectRef}
	if resultErr != nil {
		payload["reason"] = resultErr.Error()
		payload["attempts_left"] = left
	}
	s.audit(ctx, orgID, action, ResourceOTP, challenge, payload)
	return out, resultErr
}
