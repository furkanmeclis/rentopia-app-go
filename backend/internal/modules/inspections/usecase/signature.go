package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
)

// Signature limits.
const (
	maxSignaturePNG    = 2 << 20
	maxSignaturePoints = 20000
	minSignaturePoints = 8
)

// strokeDoc is the stored stroke capture: canvas size and the strokes,
// each point [x, y, pressure, t_ms].
type strokeDoc struct {
	Width   float64       `json:"width"`
	Height  float64       `json:"height"`
	Strokes [][][]float64 `json:"strokes"`
}

func checkStrokes(raw json.RawMessage) (strokeDoc, int, error) {
	var doc strokeDoc
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &doc) != nil {
		return doc, 0, validation("strokes", "invalid", "strokes are {width, height, strokes: [[[x, y, pressure, t], ...]]}")
	}
	if doc.Width <= 0 || doc.Height <= 0 || doc.Width > 10000 || doc.Height > 10000 || len(doc.Strokes) == 0 {
		return doc, 0, validation("strokes", "invalid", "a signature has a canvas size and at least one stroke")
	}
	points := 0
	for _, st := range doc.Strokes {
		for _, pt := range st {
			if len(pt) < 3 || len(pt) > 4 {
				return doc, 0, validation("strokes", "invalid", "a point is [x, y, pressure, t]")
			}
			for _, v := range pt {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return doc, 0, validation("strokes", "invalid", "points are finite numbers")
				}
			}
			if pt[2] < 0 || pt[2] > 1 {
				return doc, 0, validation("strokes", "invalid", "pressure is 0-1")
			}
			points++
		}
	}
	if points < minSignaturePoints {
		return doc, 0, validation("strokes", "too_short", "the signature is too short")
	}
	if points > maxSignaturePoints {
		return doc, 0, validation("strokes", "too_long", "the signature has too many points")
	}
	return doc, points, nil
}

// SaveSignature stores the signature canvas of a draft: the strokes with
// pressure and the PNG (private object, server-side SHA-256, server time).
// The customer reviews the form first. When the rental contract version
// shown on the tablet is named, the same signature records the contract
// approval (bound to the version's content hash) instead of a second
// signature. Signing again replaces the draft's signature.
func (s *Service) SaveSignature(ctx context.Context, orgID int64, id uuid.UUID, in model.SignatureInput) (model.Inspection, error) {
	if s.store == nil {
		return model.Inspection{}, ErrStorageUnavailable
	}
	row, r, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	if !row.CustomerReviewedAt.Valid {
		return model.Inspection{}, stateErr("review_first", "the customer reviews the form before signing")
	}
	ve := &ValidationError{}
	name := strings.TrimSpace(in.SignerName)
	if n := utf8.RuneCountInString(name); n < 2 || n > 200 {
		ve.add("signer_name", "invalid", "signer name is 2-200 characters")
	}
	role := in.SignerRole
	if role == "" {
		role = "customer"
	}
	if role != "customer" && role != "driver" {
		ve.add("signer_role", "invalid", "customer or driver")
	}
	raw := strings.TrimSpace(in.PNGBase64)
	raw = strings.TrimPrefix(raw, "data:image/png;base64,")
	png, derr := base64.StdEncoding.DecodeString(raw)
	switch {
	case derr != nil || len(png) == 0:
		ve.add("png_base64", "invalid", "a base64 PNG is required")
	case len(png) > maxSignaturePNG:
		ve.add("png_base64", "too_large", "the PNG is at most 2 MiB")
	default:
		if mime, _, ok := SniffImage(png); !ok || mime != "image/png" {
			ve.add("png_base64", "invalid_type", "the signature image is a PNG")
		}
	}
	_, points, serr := checkStrokes(in.Strokes)
	if serr != nil {
		ve.Details = append(ve.Details, serr.(*ValidationError).Details...)
	}
	if in.Contract != nil && (in.Contract.DocumentUUID == uuid.Nil || in.Contract.VersionUUID == uuid.Nil || strings.TrimSpace(in.Contract.ContentHash) == "") {
		ve.add("contract", "invalid", "the contract names its document, version and content hash")
	}
	if in.Contract != nil && s.contracts == nil {
		ve.add("contract", "unavailable", "contracts are not available")
	}
	if err := ve.orNil(); err != nil {
		return model.Inspection{}, err
	}
	facts, err := s.q.GetInspectionRentalFacts(ctx, db.GetInspectionRentalFactsParams{OrganizationID: orgID, Uuid: r.UUID})
	if err != nil {
		return model.Inspection{}, err
	}
	org, err := s.orgUUID(ctx, orgID)
	if err != nil {
		return model.Inspection{}, err
	}
	var oldKey string
	if prev, err := s.q.GetInspectionSignature(ctx, db.GetInspectionSignatureParams{OrganizationID: orgID, InspectionID: row.ID}); err == nil {
		if prev.ApprovalUuid.Valid {
			// The contract approval names this signature: it stays.
			return model.Inspection{}, stateErr("signature_bound", "the signature already approved the contract")
		}
		oldKey = prev.ObjectKey
	}
	sigUUID := uuid.New()
	key := storage.InspectionSignatureObjectKey(org, r.UUID.String(), row.Uuid.String(), sigUUID.String())
	if err := s.store.Upload(ctx, storage.File{Body: bytes.NewReader(png), Size: int64(len(png)), ContentType: "image/png", Filename: "signature.png"}, key); err != nil {
		return model.Inspection{}, err
	}
	sum := sha256.Sum256(png)
	now := s.clock()
	var sigID int64
	err = s.tx(ctx, func(q *db.Queries) error {
		if _, err := q.DeleteInspectionSignature(ctx, db.DeleteInspectionSignatureParams{OrganizationID: orgID, InspectionID: row.ID}); err != nil {
			return err
		}
		created, err := q.InsertInspectionSignature(ctx, db.InsertInspectionSignatureParams{
			OrganizationID: orgID, InspectionID: row.ID, RentalID: row.RentalID, SignerName: name, SignerRole: role,
			ObjectKey: key, Sha256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(png)), Strokes: in.Strokes,
			PointCount: int32(points), SignedAt: ts(now), DeviceSignedAt: tsPtr(in.DeviceSignedAt), CreatedBy: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		sigID = created.ID
		n, err := q.SetInspectionSignatureStatus(ctx, db.SetInspectionSignatureStatusParams{
			OrganizationID: orgID, ID: row.ID, SignatureStatus: model.SignatureSigned, SignatureNote: "",
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		return nil
	})
	if err != nil {
		s.deleteObject(ctx, key)
		if immutable(err) {
			return model.Inspection{}, stateErr("finalized", "a finalized form cannot change")
		}
		return model.Inspection{}, err
	}
	if oldKey != "" && oldKey != key {
		s.deleteObject(ctx, oldKey)
	}
	if in.Contract != nil {
		_, err := s.contracts.ApproveWithSignature(ctx, orgID, contractsusecase.SignatureApproval{
			DocumentUUID: in.Contract.DocumentUUID, VersionUUID: in.Contract.VersionUUID, ContentHash: in.Contract.ContentHash,
			ApproverName: name, SignatureID: sigID, ReservationID: facts.ReservationID,
		})
		if err != nil {
			// The customer signs the content they see: a contract that
			// changed meanwhile needs a new signature.
			_ = s.tx(ctx, func(q *db.Queries) error {
				if _, err := q.DeleteInspectionSignature(ctx, db.DeleteInspectionSignatureParams{OrganizationID: orgID, InspectionID: row.ID}); err != nil {
					return err
				}
				_, err := q.SetInspectionSignatureStatus(ctx, db.SetInspectionSignatureStatusParams{
					OrganizationID: orgID, ID: row.ID, SignatureStatus: model.SignaturePending, SignatureNote: "",
				})
				return err
			})
			s.deleteObject(ctx, key)
			return model.Inspection{}, err
		}
	}
	return s.after(ctx, orgID, id)
}
