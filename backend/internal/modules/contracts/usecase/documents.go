package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Contracts is the rentals Documents port.
var _ rentalmodel.Documents = (*Service)(nil)

// canWrite: preparing documents and recording approvals belongs to the
// contract editors and to the rental desk (handover staff).
func canWrite(ctx context.Context) bool {
	return has(ctx, rbac.PermTenantContractsWrite) || has(ctx, rbac.PermTenantRentalsWrite)
}

// changedKeys lists the template's keys whose values differ (volatile
// document keys never count).
func changedKeys(keys []string, before, after map[string]string) []string {
	out := []string{}
	for _, k := range keys {
		if model.VolatileKeys[k] {
			continue
		}
		if before[k] != after[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// prepareKind renders the document of one kind of a reservation when its
// content would change (or when it does not exist yet and create is set).
// Returns the document uuid (Nil = nothing to do) and whether a new version
// was rendered. The previous version is superseded and its approvals
// invalidated in the same transaction.
func (s *Service) prepareKind(ctx context.Context, orgID, resID int64, resUUID uuid.UUID, kind string, create bool, reason string) (uuid.UUID, bool, error) {
	tpl, err := s.q.GetActiveContractTemplate(ctx, db.GetActiveContractTemplateParams{OrganizationID: orgID, Kind: kind})
	if notFound(err) {
		if create {
			return uuid.Nil, false, ErrNoTemplate
		}
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		id, created, err := s.prepareOnce(ctx, orgID, resID, resUUID, kind, create, reason, tpl)
		if uniqueViolation(err, "") || errors.Is(err, errConcurrentVersion) {
			continue // a concurrent prepare of the same document: re-read
		}
		return id, created, err
	}
	return uuid.Nil, false, errors.New("contracts: concurrent document preparation")
}

func (s *Service) prepareOnce(ctx context.Context, orgID, resID int64, resUUID uuid.UUID, kind string, create bool, reason string, tpl db.GetActiveContractTemplateRow) (uuid.UUID, bool, error) {
	doc, err := s.q.GetLiveDocument(ctx, db.GetLiveDocumentParams{OrganizationID: orgID, ReservationID: resID, Kind: kind})
	exists := err == nil
	if err != nil && !notFound(err) {
		return uuid.Nil, false, err
	}
	if !exists && !create {
		return uuid.Nil, false, nil
	}
	snap, err := s.snapshot(ctx, orgID, resID, resUUID)
	if err != nil {
		return uuid.Nil, false, err
	}
	var prev *db.GetDocumentVersionByIDRow
	keys := []string{}
	nextVersion := int32(1)
	if exists && doc.CurrentVersionID.Valid {
		p, err := s.q.GetDocumentVersionByID(ctx, db.GetDocumentVersionByIDParams{OrganizationID: orgID, ID: doc.CurrentVersionID.Int64})
		if err != nil {
			return uuid.Nil, false, err
		}
		prev = &p
		nextVersion = p.Version + 1
		var before Snapshot
		_ = json.Unmarshal(p.DataSnapshot, &before)
		keys = changedKeys(tpl.Placeholders, before.Values, snap.Values)
		if p.TemplateID == tpl.ID && len(keys) == 0 {
			return doc.Uuid, false, nil
		}
		if p.TemplateID != tpl.ID {
			keys = append([]string{"template"}, keys...)
		}
	}
	now := s.clock()
	snap.Values["document.version"] = strconv.Itoa(int(nextVersion))
	snap.Values["document.date"] = formatDate(now)
	body, _ := fill(tpl.BodyHtml, snap.Values)
	reference := snap.Values["reservation.reference"]
	html := documentHTML(kind, snap.Values["company.legal_name"], reference, nextVersion, now, body)
	hash := contentHash(html)
	snap.Refs["template_uuid"] = tpl.Uuid
	snap.Refs["template_version"] = tpl.Version
	snap.Refs["reason"] = reason
	var rentalID pgtype.Int8
	if r, err := s.q.GetDocumentRentalFacts(ctx, db.GetDocumentRentalFactsParams{OrganizationID: orgID, ReservationID: resID}); err == nil {
		rentalID = pgtype.Int8{Int64: r.ID, Valid: true}
	}
	docID, docUUID := doc.ID, doc.Uuid
	var versionID int64
	wasApproved := prev != nil && prev.Status == model.VersionApproved
	err = s.tx(ctx, func(q *db.Queries) error {
		if !exists {
			row, err := q.InsertDocument(ctx, db.InsertDocumentParams{
				OrganizationID: orgID, Kind: kind, ReservationID: resID, RentalID: rentalID, CreatedBy: int8Ptr(actorID(ctx)),
			})
			if err != nil {
				return err
			}
			docID, docUUID = row.ID, row.Uuid
		}
		v, err := q.InsertDocumentVersion(ctx, db.InsertDocumentVersionParams{
			OrganizationID: orgID, DocumentID: docID, TemplateID: tpl.ID, TemplateVersion: tpl.Version, Html: html,
			ContentHash: hash, DataSnapshot: mustJSON(snap), ChangedKeys: keys, CreatedBy: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		if v.Version != nextVersion {
			return errConcurrentVersion
		}
		versionID = v.ID
		if prev != nil {
			if _, err := q.SupersedeDocumentVersion(ctx, db.SupersedeDocumentVersionParams{OrganizationID: orgID, ID: prev.ID, SupersededAt: ts(now)}); err != nil {
				return err
			}
			if err := q.InvalidateDocumentApprovals(ctx, db.InvalidateDocumentApprovalsParams{
				OrganizationID: orgID, DocumentVersionID: prev.ID, InvalidatedAt: ts(now),
				InvalidatedReason: "superseded by version " + strconv.Itoa(int(nextVersion)) + ": " + strings.Join(keys, ", "),
			}); err != nil {
				return err
			}
			// An open remote approval link names the old content: it stops
			// working with the new version (TEC-711).
			if _, err := q.CloseOpenDocumentApprovalLinks(ctx, db.CloseOpenDocumentApprovalLinksParams{
				Status: model.LinkSuperseded, ClosedAt: ts(now), Reason: "superseded by version " + strconv.Itoa(int(nextVersion)),
				OrganizationID: orgID, DocumentID: docID, ExceptVersionID: pgtype.Int8{Int64: v.ID, Valid: true},
			}); err != nil {
				return err
			}
		}
		n, err := q.SetDocumentCurrent(ctx, db.SetDocumentCurrentParams{
			OrganizationID: orgID, ID: docID, CurrentVersionID: pgtype.Int8{Int64: v.ID, Valid: true},
			Status: model.DocumentDraft, RentalID: rentalID,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return errConcurrentVersion
		}
		data := map[string]any{
			"document_uuid": docUUID, "kind": kind, "version": v.Version, "changed_keys": keys,
			"reapproval_required": wasApproved, "content_hash": hash,
		}
		if err := q.InsertReservationEvent(ctx, db.InsertReservationEventParams{
			OrganizationID: orgID, ReservationID: resID, Action: "document_version", Reason: reason,
			Data: mustJSON(data), ActorID: int8Ptr(actorID(ctx)),
		}); err != nil {
			return err
		}
		if wasApproved && rentalID.Valid {
			return q.InsertRentalEvent(ctx, db.InsertRentalEventParams{
				OrganizationID: orgID, RentalID: rentalID.Int64, Action: rentalmodel.EventReapprovalRequired, Reason: reason,
				Data: mustJSON(data), ActorID: int8Ptr(actorID(ctx)),
			})
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, false, err
	}
	s.requestPDF(ctx, orgID, versionID)
	s.publish(ctx, model.ResourceDocuments, "updated", docUUID)
	return docUUID, true, nil
}

// errConcurrentVersion: another version was rendered in between (retried).
var errConcurrentVersion = errors.New("contracts: concurrent version")

// Prepare implements the rentals Documents port: kinds create or refresh
// those documents (skipped when no template of the kind is active); no
// kinds refresh every live document of the reservation. Returns the
// documents that got a new version.
func (s *Service) Prepare(ctx context.Context, orgID int64, reservationID int64, kinds []string, reason string) ([]uuid.UUID, error) {
	res, err := s.q.GetDocumentReservationByID(ctx, db.GetDocumentReservationByIDParams{OrganizationID: orgID, ID: reservationID})
	if err != nil {
		return nil, err
	}
	create := len(kinds) > 0
	if !create {
		kinds, err = s.q.ListLiveDocumentKinds(ctx, db.ListLiveDocumentKindsParams{OrganizationID: orgID, ReservationID: reservationID})
		if err != nil {
			return nil, err
		}
	}
	out := []uuid.UUID{}
	for _, kind := range kinds {
		id, created, err := s.prepareKind(ctx, orgID, res.ID, res.Uuid, kind, create, reason)
		if errors.Is(err, ErrNoTemplate) {
			continue
		}
		if err != nil {
			return out, err
		}
		if created {
			out = append(out, id)
		}
	}
	return out, nil
}

// ContractState implements the rentals Documents port.
func (s *Service) ContractState(ctx context.Context, orgID int64, reservationID int64) (rentalmodel.ContractState, error) {
	out := rentalmodel.ContractState{}
	_, err := s.q.GetActiveContractTemplate(ctx, db.GetActiveContractTemplateParams{OrganizationID: orgID, Kind: model.KindRentalContract})
	if notFound(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Required = true
	doc, err := s.q.GetLiveDocument(ctx, db.GetLiveDocumentParams{OrganizationID: orgID, ReservationID: reservationID, Kind: model.KindRentalContract})
	if notFound(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	id := doc.Uuid
	out.DocumentUUID = &id
	out.Approved = doc.Status == model.DocumentApproved
	return out, nil
}

// Void implements the rentals Documents port (rental cancelled).
func (s *Service) Void(ctx context.Context, orgID int64, rentalID int64) error {
	ids, err := s.q.VoidRentalDocuments(ctx, db.VoidRentalDocumentsParams{OrganizationID: orgID, RentalID: pgtype.Int8{Int64: rentalID, Valid: true}})
	if err != nil {
		return err
	}
	s.publish(ctx, model.ResourceDocuments, "updated", ids...)
	return nil
}

// ----------------------------------------------------------------- HTTP

func (s *Service) documentRow(ctx context.Context, orgID int64, id uuid.UUID) (db.GetDocumentRow, error) {
	row, err := s.q.GetDocument(ctx, db.GetDocumentParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrDocumentNotFound
	}
	if err != nil {
		return row, err
	}
	if !inScope(ctx, row.PickupBranchID, row.ReturnBranchID, row.ActualReturnBranchID) {
		return row, ErrDocumentNotFound
	}
	return row, nil
}

// PrepareDocument creates or refreshes the document of a kind for a
// reservation (contracts.write or rentals.write).
func (s *Service) PrepareDocument(ctx context.Context, orgID int64, in model.PrepareInput) (model.PrepareResult, error) {
	if !canWrite(ctx) {
		return model.PrepareResult{}, ErrForbidden
	}
	ve := &ValidationError{}
	if !model.Contains(model.Kinds, in.Kind) {
		ve.add("kind", "invalid", "unknown document kind")
	}
	if in.ReservationUUID == nil {
		ve.add("reservation_uuid", "required", "reservation_uuid is required")
	}
	if err := ve.orNil(); err != nil {
		return model.PrepareResult{}, err
	}
	res, err := s.q.GetDocumentReservationRef(ctx, db.GetDocumentReservationRefParams{OrganizationID: orgID, Uuid: *in.ReservationUUID})
	if notFound(err) || (err == nil && !inScope(ctx, res.PickupBranchID, res.ReturnBranchID)) {
		return model.PrepareResult{}, validation("reservation_uuid", "not_found", "reservation was not found")
	}
	if err != nil {
		return model.PrepareResult{}, err
	}
	id, created, err := s.prepareKind(ctx, orgID, res.ID, res.Uuid, in.Kind, true, "manual")
	if err != nil {
		return model.PrepareResult{}, err
	}
	doc, err := s.GetDocument(ctx, orgID, id)
	return model.PrepareResult{Document: doc, Created: created}, err
}

// RefreshDocument re-renders a document when its data changed.
func (s *Service) RefreshDocument(ctx context.Context, orgID int64, id uuid.UUID) (model.PrepareResult, error) {
	row, err := s.documentRow(ctx, orgID, id)
	if err != nil {
		return model.PrepareResult{}, err
	}
	if !canWrite(ctx) {
		return model.PrepareResult{}, ErrForbidden
	}
	if row.Status == model.DocumentVoid {
		return model.PrepareResult{}, stateErr("void", "the document was withdrawn")
	}
	_, created, err := s.prepareKind(ctx, orgID, row.ReservationID, row.ReservationUuid, row.Kind, false, "manual")
	if err != nil {
		return model.PrepareResult{}, err
	}
	doc, err := s.GetDocument(ctx, orgID, id)
	return model.PrepareResult{Document: doc, Created: created}, err
}

func versionInfo(id uuid.UUID, version int32, status string, tplUUID pgtype.UUID, tplName string, tplVersion int32, hash string,
	keys []string, pdfStatus, pdfError string, pdfSize int64, issued, superseded, created pgtype.Timestamptz, by *model.Actor) model.VersionInfo {
	v := model.VersionInfo{
		UUID: id, Version: version, Status: status, TemplateName: tplName, TemplateVersion: tplVersion, ContentHash: hash,
		ChangedKeys: keys, PDFStatus: pdfStatus, PDFError: pdfError, PDFSizeBytes: pdfSize, IssuedAt: timePtr(issued),
		SupersededAt: timePtr(superseded), CreatedBy: by, CreatedAt: created.Time.UTC(),
	}
	if tplUUID.Valid {
		t := uuid.UUID(tplUUID.Bytes)
		v.TemplateUUID = &t
	}
	if v.ChangedKeys == nil {
		v.ChangedKeys = []string{}
	}
	return v
}

// GetDocument returns a document with its versions and approvals.
func (s *Service) GetDocument(ctx context.Context, orgID int64, id uuid.UUID) (model.Document, error) {
	row, err := s.documentRow(ctx, orgID, id)
	if err != nil {
		return model.Document{}, err
	}
	versions, err := s.q.ListDocumentVersions(ctx, db.ListDocumentVersionsParams{OrganizationID: orgID, DocumentID: row.ID})
	if err != nil {
		return model.Document{}, err
	}
	approvals, err := s.q.ListDocumentApprovals(ctx, db.ListDocumentApprovalsParams{OrganizationID: orgID, DocumentID: row.ID})
	if err != nil {
		return model.Document{}, err
	}
	out := model.Document{
		UUID: row.Uuid, Kind: row.Kind, Status: row.Status, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		Reservation: model.Ref{UUID: row.ReservationUuid, Reference: row.ReservationReference, Status: row.ReservationStatus},
		Versions:    []model.VersionInfo{}, Approvals: []model.Approval{},
	}
	if row.RentalUuid.Valid {
		out.Rental = &model.Ref{UUID: row.RentalUuid.Bytes, Reference: row.RentalReference.String, Status: row.RentalStatus.String}
	}
	for _, v := range versions {
		vi := versionInfo(v.Uuid, v.Version, v.Status, pgtype.UUID{Bytes: v.TemplateUuid, Valid: true}, v.TemplateName, v.TemplateVersion,
			v.ContentHash, v.ChangedKeys, v.PdfStatus, v.PdfError, v.PdfSizeBytes, v.IssuedAt, v.SupersededAt, v.CreatedAt,
			actorOf(v.CreatedByUuid, v.CreatedByName, v.CreatedBySurname, v.CreatedByEmail))
		out.Versions = append(out.Versions, vi)
		if row.CurrentVersionID.Valid && v.ID == row.CurrentVersionID.Int64 {
			cur := vi
			out.Current = &cur
		}
	}
	for _, a := range approvals {
		ap := model.Approval{
			UUID: a.Uuid, VersionUUID: a.VersionUuid, VersionNumber: a.VersionNumber, ContentHash: a.ContentHash, Party: a.Party,
			Method: a.Method, ApproverName: a.ApproverName, Note: a.Note, ApprovedAt: a.ApprovedAt.Time.UTC(),
			Valid: !a.InvalidatedAt.Valid, InvalidatedAt: timePtr(a.InvalidatedAt), InvalidatedReason: a.InvalidatedReason,
			RecordedBy:          actorOf(a.RecordedByUuid, a.RecordedByName, a.RecordedBySurname, a.RecordedByEmail),
			VerifiedPhoneMasked: a.VerifiedPhoneMasked, SignerRole: a.SignerRole.String, UserAgent: a.UserAgent,
		}
		if a.LinkUuid.Valid {
			l := uuid.UUID(a.LinkUuid.Bytes)
			ap.LinkUUID = &l
		}
		if a.IpAddress != nil {
			ap.IPAddress = a.IpAddress.String()
		}
		out.Approvals = append(out.Approvals, ap)
	}
	return out, nil
}

// ListDocuments returns a page of documents in the caller's branch scope.
func (s *Service) ListDocuments(ctx context.Context, orgID int64, f model.DocumentFilter) ([]model.DocumentListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	rows, err := s.q.ListDocuments(ctx, db.ListDocumentsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Kinds: nilIfEmpty(f.Kinds), Statuses: nilIfEmpty(f.Statuses),
		ReservationUuids: nilUUIDs(f.ReservationUUIDs), RentalUuids: nilUUIDs(f.RentalUUIDs), Q: textOf(strings.TrimSpace(f.Q)),
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.DocumentListItem, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		it := model.DocumentListItem{
			UUID: r.Uuid, Kind: r.Kind, Status: r.Status, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
			Reservation:   model.Ref{UUID: r.ReservationUuid, Reference: r.ReservationReference},
			VersionNumber: r.VersionNumber.Int32, VersionStatus: r.VersionStatus.String, ContentHash: r.ContentHash.String,
			PDFStatus: r.PdfStatus.String, TemplateName: r.TemplateName.String, TemplateVersion: r.TemplateVersion.Int32,
			VersionCreatedAt: timePtr(r.VersionCreatedAt),
			RemoteLinkStatus: effectiveLinkStatus(r.RemoteLinkStatus, r.RemoteLinkExpiresAt, s.clock()),
		}
		if r.RentalUuid.Valid {
			it.Rental = &model.Ref{UUID: r.RentalUuid.Bytes, Reference: r.RentalReference.String}
		}
		if r.VersionUuid.Valid {
			v := uuid.UUID(r.VersionUuid.Bytes)
			it.VersionUUID = &v
		}
		out = append(out, it)
	}
	return out, total, nil
}

func nilUUIDs(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}

func (s *Service) versionRow(ctx context.Context, orgID int64, doc db.GetDocumentRow, id uuid.UUID) (db.GetDocumentVersionRow, error) {
	v, err := s.q.GetDocumentVersion(ctx, db.GetDocumentVersionParams{OrganizationID: orgID, DocumentID: doc.ID, Uuid: id})
	if notFound(err) {
		return v, ErrVersionNotFound
	}
	return v, err
}

// GetVersion returns one version with its HTML and data snapshot.
func (s *Service) GetVersion(ctx context.Context, orgID int64, docID, id uuid.UUID) (model.Version, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.Version{}, err
	}
	v, err := s.versionRow(ctx, orgID, doc, id)
	if err != nil {
		return model.Version{}, err
	}
	return model.Version{
		VersionInfo: versionInfo(v.Uuid, v.Version, v.Status, pgtype.UUID{Bytes: v.TemplateUuid, Valid: true}, v.TemplateName,
			v.TemplateVersion, v.ContentHash, v.ChangedKeys, v.PdfStatus, v.PdfError, v.PdfSizeBytes, v.IssuedAt, v.SupersededAt,
			v.CreatedAt, actorOf(v.CreatedByUuid, v.CreatedByName, v.CreatedBySurname, v.CreatedByEmail)),
		HTML: v.Html, DataSnapshot: json.RawMessage(v.DataSnapshot), PDFSHA256: v.PdfSha256.String,
	}, nil
}

// IssueVersion marks the current draft as issued (final, handed to the
// customer for approval) and renders its PDF.
func (s *Service) IssueVersion(ctx context.Context, orgID int64, docID, id uuid.UUID) (model.Document, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.Document{}, err
	}
	v, err := s.versionRow(ctx, orgID, doc, id)
	if err != nil {
		return model.Document{}, err
	}
	if !canWrite(ctx) {
		return model.Document{}, ErrForbidden
	}
	if !doc.CurrentVersionID.Valid || doc.CurrentVersionID.Int64 != v.ID || v.Status != model.VersionDraft {
		return model.Document{}, stateErr("not_current_draft", "only the current draft version can be issued")
	}
	n, err := s.q.IssueDocumentVersion(ctx, db.IssueDocumentVersionParams{OrganizationID: orgID, ID: v.ID, IssuedAt: ts(s.clock()), IssuedBy: int8Ptr(actorID(ctx))})
	if err != nil {
		return model.Document{}, err
	}
	if n != 1 {
		return model.Document{}, stateErr("not_current_draft", "only the current draft version can be issued")
	}
	s.requestPDF(ctx, orgID, v.ID)
	s.publish(ctx, model.ResourceDocuments, "updated", docID)
	return s.GetDocument(ctx, orgID, docID)
}

// ApproveVersion records the approval of the current version. The caller
// sends the content hash it showed: an approval binds to that exact
// content (a version rendered since answers CONTENT_CHANGED). TEC-710
// records in-person approvals; click / signature flows come with TEC-711.
func (s *Service) ApproveVersion(ctx context.Context, orgID int64, docID, id uuid.UUID, in model.ApproveInput) (model.Document, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return model.Document{}, err
	}
	v, err := s.versionRow(ctx, orgID, doc, id)
	if err != nil {
		return model.Document{}, err
	}
	if !canWrite(ctx) {
		return model.Document{}, ErrForbidden
	}
	ve := &ValidationError{}
	party := in.Party
	if party == "" {
		party = model.PartyCustomer
	}
	if party != model.PartyCustomer && party != model.PartyStaff {
		ve.add("party", "invalid", "party is customer or staff")
	}
	method := in.Method
	if method == "" {
		method = model.MethodInPerson
	}
	if method != model.MethodInPerson {
		ve.add("method", "unsupported", "only in-person approvals are recorded here (signing flows: TEC-711)")
	}
	name := strings.TrimSpace(in.ApproverName)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 200 {
		ve.add("approver_name", "invalid", "approver name is 2-200 characters")
	}
	if len(in.Note) > 1000 {
		ve.add("note", "too_long", "note is at most 1000 characters")
	}
	hash := strings.ToLower(strings.TrimSpace(in.ContentHash))
	if hash == "" {
		ve.add("content_hash", "required", "content_hash of the approved content is required")
	}
	if err := ve.orNil(); err != nil {
		return model.Document{}, err
	}
	if doc.Status == model.DocumentVoid {
		return model.Document{}, stateErr("void", "the document was withdrawn")
	}
	if !doc.CurrentVersionID.Valid || doc.CurrentVersionID.Int64 != v.ID || v.Status == model.VersionSuperseded {
		return model.Document{}, &ContentChangedError{Current: currentUUID(ctx, s, orgID, doc)}
	}
	if hash != v.ContentHash {
		return model.Document{}, &ContentChangedError{Current: &v.Uuid}
	}
	if v.Status == model.VersionApproved {
		return model.Document{}, stateErr("already_approved", "this version is already approved")
	}
	now := s.clock()
	var customer pgtype.Int8
	if party == model.PartyCustomer {
		customer = pgtype.Int8{Int64: doc.CustomerID, Valid: true}
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.ApproveDocumentVersion(ctx, db.ApproveDocumentVersionParams{
			OrganizationID: orgID, ID: v.ID, ContentHash: hash, ApprovedAt: ts(now), ActorID: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return &ContentChangedError{Current: &v.Uuid}
		}
		approval, err := q.InsertDocumentApproval(ctx, db.InsertDocumentApprovalParams{
			OrganizationID: orgID, DocumentVersionID: v.ID, ContentHash: hash, Party: party, Method: method, ApproverName: name,
			CustomerID: customer, Note: strings.TrimSpace(in.Note), RecordedBy: int8Ptr(actorID(ctx)), ApprovedAt: ts(now),
		})
		if err != nil {
			return err
		}
		if _, err := q.SetDocumentStatus(ctx, db.SetDocumentStatusParams{OrganizationID: orgID, ID: doc.ID, Status: model.DocumentApproved}); err != nil {
			return err
		}
		// The version is approved: an open remote link has nothing left
		// to approve.
		if _, err := q.CloseOpenDocumentApprovalLinks(ctx, db.CloseOpenDocumentApprovalLinksParams{
			Status: model.LinkRevoked, ClosedAt: ts(now), RevokedBy: int8Ptr(actorID(ctx)), Reason: "approved_in_person",
			OrganizationID: orgID, DocumentID: doc.ID,
		}); err != nil {
			return err
		}
		data := map[string]any{
			"document_uuid": doc.Uuid, "kind": doc.Kind, "version": v.Version, "content_hash": hash, "approval_uuid": approval,
			"approver": name, "method": method, "party": party,
		}
		action := "document_approved"
		if doc.Kind == model.KindTerms {
			action = "terms_approved"
		}
		if err := q.InsertReservationEvent(ctx, db.InsertReservationEventParams{
			OrganizationID: orgID, ReservationID: doc.ReservationID, Action: action, Data: mustJSON(data), ActorID: int8Ptr(actorID(ctx)),
		}); err != nil {
			return err
		}
		if doc.RentalID.Valid {
			return q.InsertRentalEvent(ctx, db.InsertRentalEventParams{
				OrganizationID: orgID, RentalID: doc.RentalID.Int64, Action: rentalmodel.EventDocumentApproved,
				Data: mustJSON(data), ActorID: int8Ptr(actorID(ctx)),
			})
		}
		return nil
	})
	if err != nil {
		return model.Document{}, err
	}
	s.requestPDF(ctx, orgID, v.ID)
	s.publish(ctx, model.ResourceDocuments, "updated", docID)
	if doc.RentalUuid.Valid {
		s.publish(ctx, rentalmodel.ResourceRentals, "updated", doc.RentalUuid.Bytes)
	}
	return s.GetDocument(ctx, orgID, docID)
}

func currentUUID(ctx context.Context, s *Service, orgID int64, doc db.GetDocumentRow) *uuid.UUID {
	if !doc.CurrentVersionID.Valid {
		return nil
	}
	v, err := s.q.GetDocumentVersionByID(ctx, db.GetDocumentVersionByIDParams{OrganizationID: orgID, ID: doc.CurrentVersionID.Int64})
	if err != nil {
		return nil
	}
	return &v.Uuid
}

// ContentChangedError: the approved content is not the current version
// (409 CONTENT_CHANGED with the current version uuid).
type ContentChangedError struct {
	Current *uuid.UUID
}

func (e *ContentChangedError) Error() string {
	return "the document changed since it was shown; review and approve the current version"
}
