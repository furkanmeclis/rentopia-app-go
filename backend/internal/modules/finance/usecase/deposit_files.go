package usecase

import (
	"bytes"
	"context"
	"io"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Documents of deposit deductions (a damage invoice, a repair estimate):
// private objects under organizations/{org}/deposits/…, streamed by the
// API (the browser never sees a storage key). Same file rules as expense
// receipts: PDF, JPEG, PNG or WebP up to 10 MiB, type sniffed from the
// bytes.

func (s *Service) deductionRow(ctx context.Context, orgID int64, depositID, deductionID uuid.UUID) (db.GetSecurityDepositRowRow, db.GetDepositDeductionRowRow, error) {
	d, err := s.depositRow(ctx, nil, orgID, depositID)
	if err != nil {
		return d, db.GetDepositDeductionRowRow{}, err
	}
	x, err := s.q.GetDepositDeductionRow(ctx, db.GetDepositDeductionRowParams{OrganizationID: orgID, DepositID: d.ID, Uuid: deductionID})
	if notFound(err) {
		return d, x, ErrDeductionNotFound
	}
	return d, x, err
}

// UploadDeductionDocument stores (or replaces) the document of a deduction
// (tenant.finance.deposits.deduct).
func (s *Service) UploadDeductionDocument(ctx context.Context, orgID int64, depositID, deductionID uuid.UUID, in FileUpload) (model.Deposit, error) {
	if !has(ctx, rbac.PermTenantFinanceDepositsDeduct) {
		return model.Deposit{}, ErrForbidden
	}
	if s.storage == nil {
		return model.Deposit{}, ErrStorageUnavailable
	}
	_, x, err := s.deductionRow(ctx, orgID, depositID, deductionID)
	if err != nil {
		return model.Deposit{}, err
	}
	if in.Size <= 0 || in.Body == nil {
		return model.Deposit{}, validation("file", "required", "file is required")
	}
	if in.Size > MaxAttachmentBytes {
		return model.Deposit{}, validation("file", "too_large", "file must be at most 10 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	mime, ext, ok := SniffAttachment(head)
	if !ok {
		return model.Deposit{}, validation("file", "invalid_type", "file must be a PDF, JPEG, PNG or WebP")
	}
	orgUUID, err := s.q.GetOrganizationUUIDForFinance(ctx, orgID)
	if err != nil {
		return model.Deposit{}, err
	}
	key := storage.DepositDeductionObjectKey(orgUUID.String(), depositID.String(), deductionID.String(), uuid.NewString(), ext)
	name := cleanFilename(in.Filename, ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: name,
	}, key); err != nil {
		return model.Deposit{}, err
	}
	if _, err := s.q.SetDepositDeductionDocument(ctx, db.SetDepositDeductionDocumentParams{
		OrganizationID: orgID, ID: x.ID, DocumentObjectKey: pgtype.Text{String: key, Valid: true}, DocumentName: name,
		DocumentContentType: mime, DocumentSizeBytes: in.Size,
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.Deposit{}, err
	}
	if x.DocumentObjectKey.Valid && x.DocumentObjectKey.String != key {
		_ = s.storage.Delete(ctx, x.DocumentObjectKey.String)
	}
	s.publish(ctx, model.ResourceDeposits, "updated", depositID)
	return s.GetDeposit(ctx, orgID, depositID)
}

// OpenDeductionDocument opens a deduction's document for streaming (only
// keys under the organization's prefix are served).
func (s *Service) OpenDeductionDocument(ctx context.Context, orgID int64, depositID, deductionID uuid.UUID) (AttachmentFile, error) {
	if s.storage == nil {
		return AttachmentFile{}, ErrStorageUnavailable
	}
	_, x, err := s.deductionRow(ctx, orgID, depositID, deductionID)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !x.DocumentObjectKey.Valid {
		return AttachmentFile{}, ErrNoDocument
	}
	orgUUID, err := s.q.GetOrganizationUUIDForFinance(ctx, orgID)
	if err != nil {
		return AttachmentFile{}, err
	}
	if !storage.OrganizationOwnsKey(x.DocumentObjectKey.String, orgUUID.String()) {
		s.log.Error("deposit_document_outside_organization", "deduction", deductionID.String())
		return AttachmentFile{}, ErrNoDocument
	}
	rc, size, err := s.storage.Download(ctx, x.DocumentObjectKey.String)
	if err != nil {
		s.log.Warn("deposit_document_missing", "deduction", deductionID.String(), "error", err)
		return AttachmentFile{}, ErrNoDocument
	}
	if size <= 0 {
		size = x.DocumentSizeBytes
	}
	return AttachmentFile{Body: rc, ContentType: x.DocumentContentType, Name: x.DocumentName, Size: size}, nil
}

// DeleteDeductionDocument removes a deduction's document.
func (s *Service) DeleteDeductionDocument(ctx context.Context, orgID int64, depositID, deductionID uuid.UUID) (model.Deposit, error) {
	if !has(ctx, rbac.PermTenantFinanceDepositsDeduct) {
		return model.Deposit{}, ErrForbidden
	}
	_, x, err := s.deductionRow(ctx, orgID, depositID, deductionID)
	if err != nil {
		return model.Deposit{}, err
	}
	if !x.DocumentObjectKey.Valid {
		return model.Deposit{}, ErrNoDocument
	}
	if _, err := s.q.SetDepositDeductionDocument(ctx, db.SetDepositDeductionDocumentParams{OrganizationID: orgID, ID: x.ID}); err != nil {
		return model.Deposit{}, err
	}
	if s.storage != nil {
		_ = s.storage.Delete(ctx, x.DocumentObjectKey.String)
	}
	s.publish(ctx, model.ResourceDeposits, "updated", depositID)
	return s.GetDeposit(ctx, orgID, depositID)
}
