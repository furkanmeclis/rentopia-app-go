package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	invoicesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func price(currency string, total, prepay, paid money.Money) model.Price {
	return model.Price{
		Currency: currency, Total: total, Prepayment: prepay, Paid: paid,
		PrepaymentDue: prepay.Sub(paid).ClampZero(), Balance: total.Sub(paid).ClampZero(),
	}
}

// Reservations lists the session's reservations in the organization: the
// customer's own (renter session) or the ones the driver is on (driver
// session).
func (s *Service) Reservations(ctx context.Context, p Principal) ([]model.ReservationSummary, error) {
	rows, err := s.q.ListPortalReservations(ctx, db.ListPortalReservationsParams{
		OrganizationID: p.Org.ID, CustomerID: p.CustomerID, DriverID: p.DriverID,
	})
	if err != nil {
		return nil, err
	}
	m := money.FromNumericOrZero
	out := make([]model.ReservationSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.ReservationSummary{
			UUID: r.Uuid, Reference: r.Reference, Status: r.Status, PaymentStatus: r.PaymentStatus,
			StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(), PickupBranch: r.PickupBranchName,
			ReturnBranch: r.ReturnBranchName, Product: r.ProductTitle,
			Price: price(r.Currency, m(r.TotalAmount), m(r.PrepaymentAmount), m(r.PaidAmount)),
		})
	}
	return out, nil
}

// scoped loads one reservation of the session (another customer's or
// another organization's: ErrNotFound).
func (s *Service) scoped(ctx context.Context, p Principal, id uuid.UUID) (db.GetPortalReservationRow, error) {
	row, err := s.q.GetPortalReservation(ctx, db.GetPortalReservationParams{
		OrganizationID: p.Org.ID, Uuid: id, CustomerID: p.CustomerID, DriverID: p.DriverID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// policySnapshot reads the policy part of a reservation's snapshot. The
// self-cancel flag and the cancellation fee are read when a snapshot names
// them (top level or under policies); the settings of today never do, so
// cancellation always goes through staff until a policy version adds them.
type policySnapshot struct {
	Policies struct {
		Cancellation      string `json:"cancellation"`
		NoShow            string `json:"no_show"`
		EarlyReturn       string `json:"early_return"`
		Extension         string `json:"extension"`
		Fuel              string `json:"fuel"`
		Mileage           string `json:"mileage"`
		SelfCancelAllowed *bool  `json:"self_cancel_allowed"`
		CancellationFee   any    `json:"cancellation_fee"`
	} `json:"policies"`
	SelfCancelAllowed *bool `json:"self_cancel_allowed"`
	CancellationFee   any   `json:"cancellation_fee"`
}

func parsePolicy(raw []byte) (model.Policies, model.Cancellation) {
	var snap policySnapshot
	_ = json.Unmarshal(raw, &snap)
	pol := model.Policies{
		Cancellation: snap.Policies.Cancellation, NoShow: snap.Policies.NoShow, EarlyReturn: snap.Policies.EarlyReturn,
		Extension: snap.Policies.Extension, Fuel: snap.Policies.Fuel, Mileage: snap.Policies.Mileage,
	}
	c := model.Cancellation{PolicyText: pol.Cancellation}
	self := snap.SelfCancelAllowed
	if snap.Policies.SelfCancelAllowed != nil {
		self = snap.Policies.SelfCancelAllowed
	}
	c.SelfCancel = self != nil && *self
	fee := snap.CancellationFee
	if snap.Policies.CancellationFee != nil {
		fee = snap.Policies.CancellationFee
	}
	if fee != nil {
		if v, err := money.ParseRound(strings.TrimSpace(fmt.Sprint(fee))); err == nil && !v.IsNegative() {
			c.Fee = &v
		}
	}
	return pol, c
}

// payDue is what "pay remaining" collects now: the prepayment still due
// while the reservation waits for payment, else the remaining balance of a
// confirmed reservation. Zero = nothing to pay online.
func payDue(r resmodel.Reservation) money.Money {
	switch r.Status {
	case resmodel.StatusAwaitingPayment:
		return r.Price.PrepaymentDue.ClampZero()
	case resmodel.StatusConfirmed:
		return r.Price.Balance.ClampZero()
	}
	return money.Zero()
}

func (s *Service) paymentsOn(ctx context.Context, orgID int64) bool {
	if s.payments == nil || s.accounts == nil {
		return false
	}
	_, err := s.accounts.ActiveAccount(ctx, orgID, integrations.KindPayment)
	return err == nil
}

// changeable: a change can still be asked for.
func changeable(status string) bool {
	switch status {
	case resmodel.StatusRequest, resmodel.StatusAwaitingReview, resmodel.StatusAwaitingPayment, resmodel.StatusConfirmed:
		return true
	}
	return false
}

// Reservation is the customer's view of one of the session's reservations.
func (s *Service) Reservation(ctx context.Context, p Principal, id uuid.UUID) (model.Reservation, error) {
	row, err := s.scoped(ctx, p, id)
	if err != nil {
		return model.Reservation{}, err
	}
	octx := orgContext(ctx, p.Org)
	r, err := s.reservations.Get(octx, p.Org.ID, row.Uuid)
	if err != nil {
		return model.Reservation{}, err
	}
	out := model.Reservation{
		ReservationSummary: model.ReservationSummary{
			UUID: r.UUID, Reference: r.Reference, Status: r.Status, PaymentStatus: r.PaymentStatus, StartsAt: r.StartsAt, EndsAt: r.EndsAt,
			PickupBranch: r.PickupBranch.Name, ReturnBranch: r.ReturnBranch.Name, Product: r.Plan.SoldProduct.Title,
			Price: price(r.Price.Currency, r.Price.Total, r.Price.Prepayment, r.Price.Paid),
		},
		Drivers: []string{}, Extras: []model.Extra{}, Documents: []model.Document{}, Invoices: []model.Invoice{}, Requests: []model.Request{},
		PaymentDeadline: r.PaymentDeadline,
	}
	if r.PickupPoint != nil {
		out.PickupPoint = r.PickupPoint.Name
	}
	if r.ReturnPoint != nil {
		out.ReturnPoint = r.ReturnPoint.Name
	}
	// The plate is shown once it is fixed (promised to the customer).
	if r.Plan.PlateFixed && r.Plan.AssignedVehicle != nil {
		out.Vehicle = strings.TrimSpace(r.Plan.AssignedVehicle.Plate + " " + r.Plan.AssignedVehicle.Label)
	}
	for _, d := range r.Drivers {
		if n := shortName(d.Name); n != "" {
			out.Drivers = append(out.Drivers, n)
		}
	}
	for _, l := range r.Price.Snapshot.Lines {
		if l.Kind == resmodel.LineExtra {
			out.Extras = append(out.Extras, model.Extra{Name: l.Name, Quantity: l.Quantity, Amount: l.Amount})
		}
	}
	out.Policies, out.Cancellation = parsePolicy(r.Policy)

	requests, err := s.requests(ctx, p.Org.ID, row.ID)
	if err != nil {
		return model.Reservation{}, err
	}
	out.Requests = requests
	openType := map[string]bool{}
	for _, q := range requests {
		if q.Status == model.RequestOpen {
			openType[q.Type] = true
		}
	}
	due := payDue(r)
	if due.IsPositive() && s.paymentsOn(octx, p.Org.ID) {
		out.Actions.Pay = true
		out.Actions.PayAmount = &due
	}
	out.Actions.RequestCancel = resmodel.CanAct(r.Status, resmodel.ActionCancel) && !openType[model.RequestCancel]
	out.Actions.RequestChange = changeable(r.Status) && !openType[model.RequestChange]

	docs, err := s.reservationDocuments(octx, p, r)
	if err != nil {
		return model.Reservation{}, err
	}
	out.Documents = docs
	// Invoices are the renter's financial documents: a driver session does
	// not see them.
	if s.invoices != nil && p.Role == model.RoleRenter {
		prefix := "customer/" + p.Org.Slug + "/reservations/" + row.Uuid.String()
		invs, err := s.invoices.PortalInvoices(octx, p.Org.ID, row.ID, prefix)
		if err != nil {
			return model.Reservation{}, err
		}
		for _, i := range invs {
			out.Invoices = append(out.Invoices, model.Invoice{
				UUID: i.UUID, Number: i.Number, Profile: i.Profile, Type: i.Type, IssueDate: i.IssueDate,
				Payable: i.Payable, Currency: i.Currency, DownloadPath: i.DownloadPath,
			})
		}
	}
	return out, nil
}

// InvoicePDF returns the PDF of an issued invoice of one of the renter
// session's reservations (404 for any other invoice, and for driver
// sessions).
func (s *Service) InvoicePDF(ctx context.Context, p Principal, reservationID, invoiceID uuid.UUID, client model.ClientInfo) ([]byte, string, error) {
	row, err := s.scoped(ctx, p, reservationID)
	if err != nil {
		return nil, "", err
	}
	if s.invoices == nil || p.Role != model.RoleRenter {
		return nil, "", ErrNotFound
	}
	octx := orgContext(ctx, p.Org)
	data, name, err := s.invoices.PortalInvoicePDF(octx, p.Org.ID, row.ID, invoiceID)
	if err != nil {
		if errors.Is(err, invoicesusecase.ErrNotFound) || errors.Is(err, invoicesusecase.ErrNoDocument) {
			return nil, "", ErrNotFound
		}
		if errors.Is(err, invoicesusecase.ErrPDFUnavailable) || errors.Is(err, invoicesusecase.ErrStorageUnavailable) {
			return nil, "", ErrPDFNotReady
		}
		return nil, "", err
	}
	s.audit(octx, "customer_portal.invoice_downloaded", model.ResourcePortal, &row.Uuid, map[string]any{
		"invoice_uuid": invoiceID, "session_uuid": p.SessionUUID, "role": p.Role,
	}, &client)
	return data, name, nil
}

func (s *Service) requests(ctx context.Context, orgID, reservationID int64) ([]model.Request, error) {
	rows, err := s.q.ListReservationCustomerRequests(ctx, db.ListReservationCustomerRequestsParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Request, 0, len(rows))
	for _, q := range rows {
		out = append(out, model.Request{
			UUID: q.Uuid, Type: q.RequestType, Status: q.Status, Message: q.Message, RequestedStartsAt: timePtr(q.RequestedStartsAt),
			RequestedEndsAt: timePtr(q.RequestedEndsAt), PolicyText: q.PolicyText, CancellationFee: feeOf(q.CancellationFee.Valid, q.CancellationFee),
			Currency: q.Currency, Outcome: q.Outcome, ResolutionNote: q.ResolutionNote, ResolvedAt: timePtr(q.ResolvedAt),
			CreatedAt: q.CreatedAt.Time.UTC(),
		})
	}
	return out, nil
}

// DownloadPath is the API path (relative to /v1) of a document PDF.
func DownloadPath(slug string, reservation, document uuid.UUID) string {
	return "customer/" + slug + "/reservations/" + reservation.String() + "/documents/" + document.String() + "/pdf"
}

// reservationDocuments lists the live documents of the reservation, with
// the open remote approval link of the signed-in person when one exists.
func (s *Service) reservationDocuments(ctx context.Context, p Principal, r resmodel.Reservation) ([]model.Document, error) {
	out := []model.Document{}
	if s.documents == nil {
		return out, nil
	}
	items, _, err := s.documents.ListDocuments(ctx, p.Org.ID, contractsmodel.DocumentFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, Limit: 50})
	if err != nil {
		return nil, err
	}
	for _, d := range items {
		if d.Status == contractsmodel.DocumentVoid || d.VersionUUID == nil {
			continue
		}
		doc := model.Document{
			UUID: d.UUID, Kind: d.Kind, Status: d.Status, VersionNumber: d.VersionNumber, PDFAvailable: true,
			DownloadPath: DownloadPath(p.Org.Slug, r.UUID, d.UUID), UpdatedAt: d.UpdatedAt,
		}
		if d.Status != contractsmodel.DocumentApproved && d.RemoteLinkStatus == contractsmodel.LinkPending {
			links, err := s.documents.ListApprovalLinks(ctx, p.Org.ID, d.UUID)
			if err != nil {
				return nil, err
			}
			for _, l := range links {
				if l.Status != contractsmodel.LinkPending || l.URL == "" || !s.linkFor(p, l) {
					continue
				}
				doc.ApprovalURL = l.URL
				due := l.ExpiresAt
				doc.ApprovalDue = &due
				break
			}
		}
		out = append(out, doc)
	}
	return out, nil
}

// linkFor: a remote approval link is offered to the person it was made
// for (the code goes to that person's number): the renter's link to a
// renter session, a driver's link to the session of that driver (by name;
// the link stores the signer's name).
func (s *Service) linkFor(p Principal, l contractsmodel.ApprovalLink) bool {
	if p.Role == model.RoleRenter {
		return l.SignerRole == contractsmodel.SignerRenter
	}
	return l.SignerRole == contractsmodel.SignerDriver && shortName(l.SignerName) == p.DisplayName
}

// DocumentPDF streams the current version of a document of one of the
// session's reservations.
func (s *Service) DocumentPDF(ctx context.Context, p Principal, reservationID, documentID uuid.UUID, client model.ClientInfo) (contractsusecase.PDFFile, error) {
	row, err := s.scoped(ctx, p, reservationID)
	if err != nil {
		return contractsusecase.PDFFile{}, err
	}
	if s.documents == nil {
		return contractsusecase.PDFFile{}, ErrNotFound
	}
	octx := orgContext(ctx, p.Org)
	doc, err := s.documents.GetDocument(octx, p.Org.ID, documentID)
	if errors.Is(err, contractsusecase.ErrDocumentNotFound) {
		return contractsusecase.PDFFile{}, ErrNotFound
	}
	if err != nil {
		return contractsusecase.PDFFile{}, err
	}
	if doc.Reservation.UUID != row.Uuid || doc.Status == contractsmodel.DocumentVoid || doc.Current == nil {
		return contractsusecase.PDFFile{}, ErrNotFound
	}
	f, err := s.documents.OpenPDF(octx, p.Org.ID, doc.UUID, doc.Current.UUID)
	switch {
	case errors.Is(err, contractsusecase.ErrPDFNotReady), errors.Is(err, contractsusecase.ErrStorageUnavailable):
		return contractsusecase.PDFFile{}, ErrPDFNotReady
	case errors.Is(err, contractsusecase.ErrDocumentNotFound), errors.Is(err, contractsusecase.ErrVersionNotFound):
		return contractsusecase.PDFFile{}, ErrNotFound
	case err != nil:
		return contractsusecase.PDFFile{}, err
	}
	s.audit(octx, "customer_portal.document_downloaded", model.ResourcePortal, &row.Uuid, map[string]any{
		"document_uuid": doc.UUID, "kind": doc.Kind, "version": doc.Current.Version, "session_uuid": p.SessionUUID, "role": p.Role,
	}, &client)
	return f, nil
}
