package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/jackc/pgx/v5"
)

// MaxIdempotencyKey is the longest accepted Idempotency-Key.
const MaxIdempotencyKey = 255

// StoredResponse is the answer of an earlier write with the same key.
type StoredResponse struct {
	Status int
	Body   []byte
}

func validIdempotencyKey(k string) bool {
	if k == "" || len(k) > MaxIdempotencyKey {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x21 || k[i] > 0x7e {
			return false
		}
	}
	return true
}

func requestHash(method, path string, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(method))
	_, _ = h.Write([]byte{'\n'})
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte{'\n'})
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func bodyAAD(orgID, id int64) string {
	return "public_api_idempotency:" + strconv.FormatInt(orgID, 10) + ":" + strconv.FormatInt(id, 10)
}

// BeginIdempotent claims an Idempotency-Key for a write. It returns the
// stored answer when the same request was already answered (replay), or
// the claim id the caller finishes with FinishIdempotent. The same key with
// another request is refused, and so is a key whose first request is still
// running.
func (s *Service) BeginIdempotent(ctx context.Context, c model.Credential, key, method, path string, body []byte) (*StoredResponse, int64, error) {
	if key == "" {
		return nil, 0, ErrIdempotencyKeyRequired
	}
	if !validIdempotencyKey(key) {
		return nil, 0, validation("Idempotency-Key", "invalid", "the key is 1-255 visible ASCII characters")
	}
	now := s.clock()
	hash := requestHash(method, path, body)
	if err := s.q.DeleteExpiredPublicIdempotencyKey(ctx, db.DeleteExpiredPublicIdempotencyKeyParams{
		OrganizationID: c.OrganizationID, KeyID: c.KeyID, Subject: c.SessionID, IdempotencyKey: key, Now: ts(now),
	}); err != nil {
		return nil, 0, err
	}
	id, err := s.q.ClaimPublicIdempotency(ctx, db.ClaimPublicIdempotencyParams{
		OrganizationID: c.OrganizationID, KeyID: c.KeyID, Subject: c.SessionID, IdempotencyKey: key,
		Method: method, Path: path, RequestHash: hash, ExpiresAt: ts(now.Add(s.limits.IdempotencyTTL)),
	})
	if err == nil {
		return nil, id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, err
	}
	row, err := s.q.GetPublicIdempotency(ctx, db.GetPublicIdempotencyParams{
		OrganizationID: c.OrganizationID, KeyID: c.KeyID, Subject: c.SessionID, IdempotencyKey: key,
	})
	if err != nil {
		return nil, 0, err
	}
	if row.RequestHash != hash {
		return nil, 0, ErrIdempotencyMismatch
	}
	if row.State == "completed" {
		plain, err := s.bodies.Open(row.ResponseEnc, bodyAAD(c.OrganizationID, row.ID))
		if err != nil {
			return nil, 0, err
		}
		return &StoredResponse{Status: int(row.ResponseStatus.Int32), Body: []byte(plain)}, 0, nil
	}
	n, err := s.q.TakeOverPublicIdempotency(ctx, db.TakeOverPublicIdempotencyParams{
		OrganizationID: c.OrganizationID, ID: row.ID, Now: ts(now), StaleBefore: ts(now.Add(-s.limits.IdempotencyStale)),
	})
	if err != nil {
		return nil, 0, err
	}
	if n == 0 {
		return nil, 0, ErrIdempotencyInProgress
	}
	return nil, row.ID, nil
}

// FinishIdempotent stores the answer of a claimed write. Server errors and
// rate limits free the key instead, so the client can retry.
func (s *Service) FinishIdempotent(ctx context.Context, c model.Credential, id int64, status int, body []byte) {
	if id == 0 {
		return
	}
	if status >= 500 || status == 429 {
		if err := s.q.DeletePublicIdempotency(ctx, db.DeletePublicIdempotencyParams{OrganizationID: c.OrganizationID, ID: id}); err != nil {
			s.log.Warn("public_api_idempotency_release_failed", "error", err)
		}
		return
	}
	enc, err := s.bodies.Seal(string(body), bodyAAD(c.OrganizationID, id))
	if err == nil {
		err = s.q.CompletePublicIdempotency(ctx, db.CompletePublicIdempotencyParams{
			OrganizationID: c.OrganizationID, ID: id, ResponseStatus: int32ToPg(status), ResponseEnc: enc,
		})
	}
	if err != nil {
		s.log.Warn("public_api_idempotency_store_failed", "error", err)
	}
}

// PurgeIdempotency deletes expired idempotency records (worker sweep).
func (s *Service) PurgeIdempotency(ctx context.Context) (int64, error) {
	return s.q.PurgeExpiredPublicIdempotency(ctx, ts(s.clock()))
}
