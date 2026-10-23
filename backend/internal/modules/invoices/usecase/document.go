package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/ubl"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// countryNames are the country names UBL writes for common codes.
var countryNames = map[string]string{"TR": "Türkiye", "DE": "Almanya", "GB": "Birleşik Krallık", "US": "Amerika Birleşik Devletleri"}

func countryName(code string) string {
	if n, ok := countryNames[strings.ToUpper(code)]; ok {
		return n
	}
	if code == "" {
		return "Türkiye"
	}
	return strings.ToUpper(code)
}

// supplierParty turns the supplier settings into the UBL party. A sole
// proprietor (TCKN) is written as a person: the last word of the legal name
// is the family name.
func supplierParty(sup model.Supplier) ubl.Party {
	p := ubl.Party{
		Identity: sup.TaxNumber, Name: sup.LegalName, TaxOffice: sup.TaxOffice, Street: sup.Address,
		District: sup.District, City: sup.City, PostalCode: sup.PostalCode, Country: "Türkiye", CountryCode: "TR",
		Email: sup.Email, Phone: sup.Phone,
	}
	switch len(sup.TaxNumber) {
	case 10:
		p.IdentityType = ubl.IdentityVKN
	case 11:
		p.IdentityType = ubl.IdentityTCKN
		parts := strings.Fields(sup.LegalName)
		if len(parts) >= 2 {
			p.FirstName, p.LastName = strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
		}
	}
	return p
}

// docInput is everything needed to build an invoice's UBL document.
type docInput struct {
	row      db.GetInvoiceRow
	lines    []db.ListInvoiceLinesRow
	supplier model.Supplier
	identity string
}

func (s *Service) docInput(ctx context.Context, q *db.Queries, orgID int64, row db.GetInvoiceRow) (docInput, error) {
	lines, err := q.ListInvoiceLines(ctx, db.ListInvoiceLinesParams{OrganizationID: orgID, InvoiceID: row.ID})
	if err != nil {
		return docInput{}, err
	}
	st, err := s.loadSettings(ctx, q, orgID)
	if err != nil {
		return docInput{}, err
	}
	sup, err := s.supplier(ctx, orgID, st)
	if err != nil {
		return docInput{}, err
	}
	identity, err := s.openIdentity(orgID, row.Uuid, row.ReceiverIdentityEnc)
	if err != nil {
		return docInput{}, err
	}
	return docInput{row: row, lines: lines, supplier: sup, identity: identity}, nil
}

// document composes the UBL document of an invoice with its number.
func document(in docInput, number string, issuedAt time.Time) ubl.Document {
	row := in.row
	idType := ""
	if row.ReceiverIdentityType.Valid {
		idType = row.ReceiverIdentityType.String
	}
	recv := ubl.Party{
		IdentityType: idType, Identity: in.identity, TaxOffice: row.ReceiverTaxOffice, Street: row.ReceiverAddress,
		District: row.ReceiverDistrict, City: row.ReceiverCity, PostalCode: row.ReceiverPostalCode,
		Country: countryName(row.ReceiverCountry), CountryCode: row.ReceiverCountry, Email: row.ReceiverEmail,
	}
	if idType == ubl.IdentityTCKN {
		recv.FirstName, recv.LastName = row.ReceiverFirstName, row.ReceiverLastName
		if row.ReceiverType == "corporate" {
			recv.Name = row.ReceiverName
		}
	} else {
		recv.Name = row.ReceiverName
	}
	doc := ubl.Document{
		UUID: row.Uuid.String(), Number: number, Profile: row.Profile, Type: row.InvoiceType,
		IssueDate: row.IssueDate.Time.Format(time.DateOnly), IssueTime: issuedAt.In(istanbul).Format("15:04:05"), Today: issuedAt.In(istanbul).Format(time.DateOnly),
		Currency: row.Currency, Supplier: supplierParty(in.supplier), Customer: recv,
	}
	if strings.TrimSpace(row.Note) != "" {
		doc.Notes = append(doc.Notes, row.Note)
	}
	if row.RentalReference.Valid {
		doc.Notes = append(doc.Notes, "Kiralama: "+row.RentalReference.String)
	}
	if row.InvoiceType == model.TypeIade && row.ReturnsNumber.Valid {
		doc.Returns = &ubl.Reference{Number: row.ReturnsNumber.String, IssueDate: row.ReturnsIssueDate.Time.Format(time.DateOnly)}
	}
	for _, l := range in.lines {
		net, _ := money.FromNumeric(l.NetAmount)
		vat, _ := money.FromNumeric(l.VatAmount)
		rate, _ := money.RateFromNumeric(l.VatRate)
		doc.Lines = append(doc.Lines, ubl.Line{
			Name: l.Name, Days: int(l.Days), Net: net, VAT: vat, Rate: rate,
			ExemptionCode: l.ExemptionCode, ExemptionReason: l.ExemptionReason,
		})
	}
	return doc
}

// placeholderNumber is the number a draft is checked / previewed with (the
// real number is taken when the invoice is finalized).
func placeholderNumber(series string, year int) string {
	return fmt.Sprintf("%s%04d%09d", series, year, 0)
}

func (s *Service) seriesOf(st invoiceSettings, profile string) string {
	if profile == model.ProfileEArsiv {
		return st.earsiv
	}
	return st.efatura
}

func supplierProblems(sup model.Supplier) []ubl.Problem {
	out := []ubl.Problem{}
	for _, f := range supplierMissing(sup) {
		out = append(out, ubl.Problem{Layer: "check", Field: f, Code: "required", Message: "set it in the invoice / company settings"})
	}
	return out
}

func (s *Service) saveProblems(ctx context.Context, orgID, id int64, problems []ubl.Problem) {
	if problems == nil {
		problems = []ubl.Problem{}
	}
	raw, _ := json.Marshal(problems)
	if err := s.q.SetInvoiceValidation(ctx, db.SetInvoiceValidationParams{
		OrganizationID: orgID, ID: id, ValidationErrors: raw, ValidatedAt: ts(s.clock()),
	}); err != nil {
		s.log.Warn("invoice_save_validation_failed", "error", err)
	}
}

// Validate checks a draft without numbering it: the module checks, the
// go-ubltr build, the XSD and the Schematron (e-Arşiv rules for
// EARSIVFATURA). The problems are stored on the draft and returned.
func (s *Service) Validate(ctx context.Context, orgID int64, id uuid.UUID) ([]ubl.Problem, error) {
	row, err := s.draftRow(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	in, err := s.docInput(ctx, s.q, orgID, row)
	if err != nil {
		return nil, err
	}
	st, err := s.loadSettings(ctx, s.q, orgID)
	if err != nil {
		return nil, err
	}
	problems := supplierProblems(in.supplier)
	now := s.clock()
	_, _, more, err := ubl.Validate(ctx, document(in, placeholderNumber(s.seriesOf(st, row.Profile), row.IssueDate.Time.Year()), now))
	if err != nil {
		return nil, err
	}
	problems = append(problems, more...)
	s.saveProblems(ctx, orgID, row.ID, problems)
	s.publish(ctx, resourceevents.ActionUpdated, row.Uuid.String())
	return problems, nil
}

func xmlKey(orgUUID, invoiceUUID uuid.UUID, number string) string {
	return fmt.Sprintf("%sinvoices/%s/%s.xml", storage.OrganizationPrefix(orgUUID.String()), invoiceUUID, number)
}

func pdfKey(orgUUID, invoiceUUID uuid.UUID, number string) string {
	return fmt.Sprintf("%sinvoices/%s/%s.pdf", storage.OrganizationPrefix(orgUUID.String()), invoiceUUID, number)
}

// Finalize numbers a draft, builds and validates its UBL-TR XML with
// go-ubltr, stores it privately and queues the send job — all or nothing:
// a document with problems keeps the draft unnumbered (*DocumentError with
// the problems per field) and nothing is sent. A finalized invoice answers
// as it is (a repeated request finalizes and sends nothing twice).
func (s *Service) Finalize(ctx context.Context, orgID int64, id uuid.UUID) (model.Detail, error) {
	if err := s.requireEInvoice(ctx, orgID); err != nil {
		return model.Detail{}, err
	}
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.Detail{}, err
	}
	if row.Status != model.StatusDraft {
		if row.Status == model.StatusCancelled {
			return model.Detail{}, ErrNotDraft
		}
		return s.Get(ctx, orgID, id)
	}
	if s.store == nil {
		return model.Detail{}, ErrStorageUnavailable
	}
	org, err := s.q.GetInvoiceOrganization(ctx, orgID)
	if err != nil {
		return model.Detail{}, err
	}
	var problems []ubl.Problem
	var jobID int64
	var pending bool
	now := s.clock()
	err = s.tx(ctx, func(q *db.Queries) error {
		in, err := s.docInput(ctx, q, orgID, row)
		if err != nil {
			return err
		}
		problems = supplierProblems(in.supplier)
		// The receiver type is checked again when the cached answer expired.
		flag := row.ReceiverEinvoice
		if row.CustomerID.Valid {
			if c, err := q.GetInvoiceCustomer(ctx, db.GetInvoiceCustomerParams{OrganizationID: orgID, ID: row.CustomerID.Int64}); err == nil && row.TaxpayerSource == model.TaxpayerCustomerFlag {
				flag = c.EInvoice
			}
		}
		tp, err := s.lookupTaxpayer(ctx, q, orgID, in.identity, flag)
		if err != nil {
			return err
		}
		if tp.einvoice != row.ReceiverEinvoice {
			problems = append(problems, ubl.Problem{Layer: "check", Field: "profile", Code: "taxpayer_changed",
				Message: "the receiver's e-Fatura registration changed: review the profile"})
		}
		st, err := s.loadSettings(ctx, q, orgID)
		if err != nil {
			return err
		}
		series := s.seriesOf(st, row.Profile)
		year := row.IssueDate.Time.Year()
		seq, err := q.NextInvoiceSequence(ctx, db.NextInvoiceSequenceParams{OrganizationID: orgID, Series: series, Year: int32(year)})
		if err != nil {
			return err
		}
		number := fmt.Sprintf("%s%04d%09d", series, year, seq)
		data, totals, more, err := ubl.Validate(ctx, document(in, number, now))
		if err != nil {
			return err
		}
		problems = append(problems, more...)
		if len(problems) > 0 {
			return &DocumentError{Problems: problems}
		}
		payable, _ := money.FromNumeric(row.PayableAmount)
		if totals.Payable.Minor() != payable.Minor() {
			return fmt.Errorf("invoices: built payable %s differs from the stored %s", totals.Payable, payable)
		}
		key := xmlKey(org.Uuid, row.Uuid, number)
		sum := sha256.Sum256(data)
		if err := s.store.Upload(ctx, storage.File{
			Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/xml", Filename: number + ".xml",
		}, key); err != nil {
			return fmt.Errorf("invoices: store xml: %w", err)
		}
		r, err := s.routeOf(ctx, q, orgID)
		if err != nil {
			return err
		}
		status, lastErr, hold := jobsusecase.StatusPending, "", ""
		if !r.ready {
			status, lastErr, hold = jobsusecase.StatusWaitingDependency, holdMessage(r.hold), r.hold
		}
		payload, _ := json.Marshal(map[string]any{"invoice_uuid": row.Uuid.String(), "number": number, "profile": row.Profile, "type": row.InvoiceType})
		jp := db.InsertInvoiceJobParams{
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, IntegrationAccountID: r.accountID, Provider: r.provider,
			Kind: model.JobKindSend, IdempotencyKey: orgKey(orgID) + ":einvoice:send:" + row.Uuid.String(), Payload: payload,
			RefUuid: pgtype.UUID{Bytes: row.Uuid, Valid: true}, MaxAttempts: MaxAttempts, Status: status, LastError: lastErr,
		}
		if status == jobsusecase.StatusPending {
			jp.NextAttemptAt = ts(now)
		}
		job, err := q.InsertInvoiceJob(ctx, jp)
		if err != nil {
			return err
		}
		n, err := q.FinalizeInvoice(ctx, db.FinalizeInvoiceParams{
			OrganizationID: orgID, ID: row.ID, Series: pgtype.Text{String: series, Valid: true},
			SequenceYear: pgtype.Int4{Int32: int32(year), Valid: true}, SequenceNo: pgtype.Int8{Int64: seq, Valid: true},
			Number: pgtype.Text{String: number, Valid: true}, XmlKey: pgtype.Text{String: key, Valid: true},
			XmlSha256: hex.EncodeToString(sum[:]), IntegrationJobID: pgtype.Int8{Int64: job.ID, Valid: true},
			HoldReason: hold, LastError: lastErr, FinalizedAt: ts(now),
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotDraft
		}
		jobID, pending = job.ID, status == jobsusecase.StatusPending
		return nil
	})
	var de *DocumentError
	switch {
	case errors.As(err, &de):
		s.saveProblems(ctx, orgID, row.ID, de.Problems)
		s.publish(ctx, resourceevents.ActionUpdated, row.Uuid.String())
		return model.Detail{}, err
	case errors.Is(err, ErrNotDraft):
		return s.Get(ctx, orgID, id)
	case err != nil:
		return model.Detail{}, err
	}
	s.publish(ctx, resourceevents.ActionUpdated, row.Uuid.String())
	if pending && s.runner != nil {
		if _, err := s.runner.Kick(orgctx.WithOrganizationID(ctx, orgID), orgID, jobID); err != nil {
			s.log.Warn("invoice_first_step_failed", "invoice", row.Uuid, "error", err)
		}
	} else if s.runner != nil {
		s.runner.Changed(ctx, orgID, jobID)
	}
	return s.Get(ctx, orgID, id)
}

func (s *Service) read(ctx context.Context, orgUUID uuid.UUID, key string) ([]byte, error) {
	if s.store == nil {
		return nil, ErrStorageUnavailable
	}
	if !storage.OrganizationOwnsKey(key, orgUUID.String()) {
		return nil, ErrNotFound
	}
	rc, _, err := s.store.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(io.LimitReader(rc, 32<<20))
}

// XML returns the stored UBL-TR document and its file name.
func (s *Service) XML(ctx context.Context, orgID int64, id uuid.UUID) ([]byte, string, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, "", err
	}
	return s.xmlOf(ctx, orgID, row.XmlKey, row.Number)
}

func (s *Service) xmlOf(ctx context.Context, orgID int64, key, number pgtype.Text) ([]byte, string, error) {
	if !key.Valid || !number.Valid {
		return nil, "", ErrNoDocument
	}
	org, err := s.q.GetInvoiceOrganization(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	data, err := s.read(ctx, org.Uuid, key.String)
	return data, number.String + ".xml", err
}

// PDF returns the invoice PDF: rendered from the stored XML with go-ubltr
// (HTML) and headless Chromium once, then kept in storage.
func (s *Service) PDF(ctx context.Context, orgID int64, id uuid.UUID) ([]byte, string, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, "", err
	}
	return s.pdfOf(ctx, orgID, row.ID, row.Uuid, row.XmlKey, row.PdfKey, row.Number)
}

func (s *Service) pdfOf(ctx context.Context, orgID, rowID int64, invUUID uuid.UUID, xml, pdfK, number pgtype.Text) ([]byte, string, error) {
	if !xml.Valid || !number.Valid {
		return nil, "", ErrNoDocument
	}
	org, err := s.q.GetInvoiceOrganization(ctx, orgID)
	if err != nil {
		return nil, "", err
	}
	name := number.String + ".pdf"
	if pdfK.Valid {
		if data, err := s.read(ctx, org.Uuid, pdfK.String); err == nil {
			return data, name, nil
		}
	}
	data, err := s.read(ctx, org.Uuid, xml.String)
	if err != nil {
		return nil, "", err
	}
	pdf, err := ubl.PDF(ctx, s.pdf, data)
	if errors.Is(err, pdfrender.ErrUnavailable) {
		return nil, "", ErrPDFUnavailable
	}
	if err != nil {
		return nil, "", err
	}
	key := pdfKey(org.Uuid, invUUID, number.String)
	if err := s.store.Upload(ctx, storage.File{
		Body: bytes.NewReader(pdf), Size: int64(len(pdf)), ContentType: "application/pdf", Filename: name,
	}, key); err == nil {
		if err := s.q.SetInvoicePDF(ctx, db.SetInvoicePDFParams{OrganizationID: orgID, ID: rowID, PdfKey: pgtype.Text{String: key, Valid: true}}); err != nil {
			s.log.Warn("invoice_pdf_key_failed", "error", err)
		}
	} else {
		s.log.Warn("invoice_pdf_store_failed", "error", err)
	}
	return pdf, name, nil
}

// Preview renders the invoice as HTML (go-ubltr render): the stored
// document of a finalized invoice, or the draft built with a placeholder
// number. A draft that cannot be built yet answers *DocumentError.
func (s *Service) Preview(ctx context.Context, orgID int64, id uuid.UUID) ([]byte, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	var data []byte
	if row.XmlKey.Valid {
		data, _, err = s.xmlOf(ctx, orgID, row.XmlKey, row.Number)
		if err != nil {
			return nil, err
		}
	} else {
		in, err := s.docInput(ctx, s.q, orgID, row)
		if err != nil {
			return nil, err
		}
		st, err := s.loadSettings(ctx, s.q, orgID)
		if err != nil {
			return nil, err
		}
		doc := document(in, placeholderNumber(s.seriesOf(st, row.Profile), row.IssueDate.Time.Year()), s.clock())
		doc.Notes = append([]string{"TASLAK — numaralandırılmamış, gönderilmemiş bir fatura önizlemesidir."}, doc.Notes...)
		data, _, err = ubl.Build(doc)
		if err != nil {
			return nil, &DocumentError{Problems: []ubl.Problem{{Layer: "builder", Field: "document", Code: "invalid", Message: err.Error()}}}
		}
	}
	return ubl.HTML(ctx, data)
}
