// Package repository is the Postgres access path of the customers resource.
// Every method takes the organization id explicitly and every query on a
// tenant table filters by it: a uuid of another organization reads as not
// found. Values arrive here already encrypted / masked by the use case.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrIdentityTaken is returned when a live customer of the organization
	// already has the identity number.
	ErrIdentityTaken = errors.New("identity taken")
	// ErrLicenceTaken is returned when a live driver of the customer already
	// has the licence number.
	ErrLicenceTaken = errors.New("licence taken")
	// ErrConsentOpen is returned when the channel already has an open grant.
	ErrConsentOpen = errors.New("consent already granted")
)

// CustomerFields are the writable columns of a customer.
type CustomerFields struct {
	CustomerType      string
	FirstName         string
	LastName          string
	CompanyTitle      string
	IdentityType      pgtype.Text
	IdentityEnc       string
	IdentityBidx      string
	IdentityHint      string
	Nationality       pgtype.Text
	TaxOffice         string
	Phone             string
	Email             string
	BillingAddress    string
	BillingCity       string
	BillingDistrict   string
	BillingPostalCode string
	BillingCountry    pgtype.Text
	EInvoice          bool
	Note              string
	Tags              []string
}

// FieldsFromCustomer copies the writable columns of a row.
func FieldsFromCustomer(c db.Customer) CustomerFields {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	return CustomerFields{
		CustomerType: c.CustomerType, FirstName: c.FirstName, LastName: c.LastName, CompanyTitle: c.CompanyTitle,
		IdentityType: c.IdentityType, IdentityEnc: c.IdentityEnc, IdentityBidx: c.IdentityBidx,
		IdentityHint: c.IdentityHint, Nationality: c.Nationality, TaxOffice: c.TaxOffice, Phone: c.Phone,
		Email: c.Email, BillingAddress: c.BillingAddress, BillingCity: c.BillingCity,
		BillingDistrict: c.BillingDistrict, BillingPostalCode: c.BillingPostalCode,
		BillingCountry: c.BillingCountry, EInvoice: c.EInvoice, Note: c.Note, Tags: tags,
	}
}

// DriverFields are the writable columns of a driver.
type DriverFields struct {
	IsCustomerSelf   bool
	FirstName        string
	LastName         string
	BirthDate        pgtype.Date
	Phone            string
	Email            string
	IdentityType     pgtype.Text
	IdentityEnc      string
	IdentityBidx     string
	IdentityHint     string
	Nationality      pgtype.Text
	LicenceEnc       string
	LicenceBidx      string
	LicenceHint      string
	LicenceClass     string
	LicenceIssuedOn  pgtype.Date
	LicenceExpiresOn pgtype.Date
	LicenceCountry   pgtype.Text
	Note             string
}

// FieldsFromDriver copies the writable columns of a row.
func FieldsFromDriver(d db.Driver) DriverFields {
	return DriverFields{
		IsCustomerSelf: d.IsCustomerSelf, FirstName: d.FirstName, LastName: d.LastName, BirthDate: d.BirthDate,
		Phone: d.Phone, Email: d.Email, IdentityType: d.IdentityType, IdentityEnc: d.IdentityEnc,
		IdentityBidx: d.IdentityBidx, IdentityHint: d.IdentityHint, Nationality: d.Nationality,
		LicenceEnc: d.LicenceEnc, LicenceBidx: d.LicenceBidx, LicenceHint: d.LicenceHint,
		LicenceClass: d.LicenceClass, LicenceIssuedOn: d.LicenceIssuedOn, LicenceExpiresOn: d.LicenceExpiresOn,
		LicenceCountry: d.LicenceCountry, Note: d.Note,
	}
}

// ListRow is a customer with its open consent channels and driver count.
type ListRow = db.ListCustomersRow

// SearchRow is the search document source of a customer.
type SearchRow = db.GetCustomerSearchDocumentRow

// DocumentRow is a customer document with its driver.
type DocumentRow = db.ListCustomerDocumentsRow

// ConsentRow is a consent record with its actors.
type ConsentRow = db.ListCustomerConsentsRow

// Postgres implements the customers repository with sqlc queries.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil when the caller never
// opens transactions (InTx then runs fn without one).
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// InTx runs fn with a repository bound to one transaction.
func (r *Postgres) InTx(ctx context.Context, fn func(tx *Postgres) error) error {
	if r.pool == nil {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("customers: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Postgres{q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- customers ------------------------------------------------------------------

// ListParams are the resolved list filters (blind indexes computed by the
// use case).
type ListParams struct {
	model.ListFilter
	QDigits       string
	IdentityBidxs []string
}

func (r *Postgres) ListCustomers(ctx context.Context, orgID int64, p ListParams) ([]ListRow, int64, error) {
	rows, err := r.q.ListCustomers(ctx, db.ListCustomersParams{
		OrganizationID: orgID, CustomerTypes: stringsArg(p.Types), Nationalities: stringsArg(p.Nationalities),
		Tags: stringsArg(p.Tags), Anonymized: boolArg(p.Anonymized), Consents: stringsArg(p.Consents),
		Q: textArg(p.Q), QDigits: textArg(p.QDigits), IdentityBidxs: stringsArg(p.IdentityBidxs),
		SortField: p.SortField, SortDesc: p.SortDesc, LimitCount: p.Limit, OffsetCount: p.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountCustomers(ctx, db.CountCustomersParams{
		OrganizationID: orgID, CustomerTypes: stringsArg(p.Types), Nationalities: stringsArg(p.Nationalities),
		Tags: stringsArg(p.Tags), Anonymized: boolArg(p.Anonymized), Consents: stringsArg(p.Consents),
		Q: textArg(p.Q), QDigits: textArg(p.QDigits), IdentityBidxs: stringsArg(p.IdentityBidxs),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) ListCustomerUUIDs(ctx context.Context, orgID int64, p ListParams, limit int32) ([]uuid.UUID, error) {
	return r.q.ListCustomerUUIDs(ctx, db.ListCustomerUUIDsParams{
		OrganizationID: orgID, CustomerTypes: stringsArg(p.Types), Nationalities: stringsArg(p.Nationalities),
		Tags: stringsArg(p.Tags), Anonymized: boolArg(p.Anonymized), Consents: stringsArg(p.Consents),
		Q: textArg(p.Q), QDigits: textArg(p.QDigits), IdentityBidxs: stringsArg(p.IdentityBidxs),
		LimitCount: limit,
	})
}

func (r *Postgres) Facets(ctx context.Context, orgID int64) ([]db.ListCustomerFacetsRow, error) {
	return r.q.ListCustomerFacets(ctx, orgID)
}

func (r *Postgres) GetCustomer(ctx context.Context, orgID int64, id uuid.UUID) (db.Customer, error) {
	row, err := r.q.GetCustomer(ctx, db.GetCustomerParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetCustomerByIdentity(ctx context.Context, orgID int64, bidx string) (db.Customer, error) {
	row, err := r.q.GetCustomerByIdentityIndex(ctx, db.GetCustomerByIdentityIndexParams{OrganizationID: orgID, IdentityBidx: bidx})
	return row, mapErr(err)
}

func (r *Postgres) IdentityTaken(ctx context.Context, orgID int64, bidx string, exclude *uuid.UUID) (bool, error) {
	if bidx == "" {
		return false, nil
	}
	var ex pgtype.UUID
	if exclude != nil {
		ex = pgtype.UUID{Bytes: *exclude, Valid: true}
	}
	return r.q.CustomerIdentityTaken(ctx, db.CustomerIdentityTakenParams{OrganizationID: orgID, IdentityBidx: bidx, ExcludeUuid: ex})
}

func (r *Postgres) CreateCustomer(ctx context.Context, orgID int64, f CustomerFields, actor pgtype.Int8) (db.Customer, error) {
	row, err := r.q.CreateCustomer(ctx, db.CreateCustomerParams{
		OrganizationID: orgID, CustomerType: f.CustomerType, FirstName: f.FirstName, LastName: f.LastName,
		CompanyTitle: f.CompanyTitle, IdentityType: f.IdentityType, IdentityEnc: f.IdentityEnc,
		IdentityBidx: f.IdentityBidx, IdentityHint: f.IdentityHint, Nationality: f.Nationality,
		TaxOffice: f.TaxOffice, Phone: f.Phone, Email: f.Email, BillingAddress: f.BillingAddress,
		BillingCity: f.BillingCity, BillingDistrict: f.BillingDistrict, BillingPostalCode: f.BillingPostalCode,
		BillingCountry: f.BillingCountry, EInvoice: f.EInvoice, Note: f.Note, Tags: tagsArg(f.Tags), CreatedBy: actor,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateCustomer(ctx context.Context, orgID int64, id uuid.UUID, f CustomerFields) (db.Customer, error) {
	row, err := r.q.UpdateCustomer(ctx, db.UpdateCustomerParams{
		OrganizationID: orgID, Uuid: id, CustomerType: f.CustomerType, FirstName: f.FirstName, LastName: f.LastName,
		CompanyTitle: f.CompanyTitle, IdentityType: f.IdentityType, IdentityEnc: f.IdentityEnc,
		IdentityBidx: f.IdentityBidx, IdentityHint: f.IdentityHint, Nationality: f.Nationality,
		TaxOffice: f.TaxOffice, Phone: f.Phone, Email: f.Email, BillingAddress: f.BillingAddress,
		BillingCity: f.BillingCity, BillingDistrict: f.BillingDistrict, BillingPostalCode: f.BillingPostalCode,
		BillingCountry: f.BillingCountry, EInvoice: f.EInvoice, Note: f.Note, Tags: tagsArg(f.Tags),
	})
	return row, mapErr(err)
}

func (r *Postgres) SetTags(ctx context.Context, orgID int64, id uuid.UUID, tags []string) (db.Customer, error) {
	row, err := r.q.SetCustomerTags(ctx, db.SetCustomerTagsParams{OrganizationID: orgID, Uuid: id, Tags: tagsArg(tags)})
	return row, mapErr(err)
}

func (r *Postgres) DeleteCustomer(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.DeleteCustomer(ctx, db.DeleteCustomerParams{OrganizationID: orgID, Uuid: id})
	return affected(n, err)
}

func (r *Postgres) AnonymizeCustomer(ctx context.Context, orgID, customerID int64) error {
	n, err := r.q.AnonymizeCustomer(ctx, db.AnonymizeCustomerParams{OrganizationID: orgID, ID: customerID})
	return affected(n, err)
}

func (r *Postgres) SearchDocument(ctx context.Context, orgID int64, id uuid.UUID) (SearchRow, error) {
	row, err := r.q.GetCustomerSearchDocument(ctx, db.GetCustomerSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

// --- contacts -------------------------------------------------------------------

func (r *Postgres) ListContacts(ctx context.Context, orgID, customerID int64) ([]db.CustomerContact, error) {
	return r.q.ListCustomerContacts(ctx, db.ListCustomerContactsParams{OrganizationID: orgID, CustomerID: customerID})
}

func (r *Postgres) GetContact(ctx context.Context, orgID, customerID int64, id uuid.UUID) (db.CustomerContact, error) {
	row, err := r.q.GetCustomerContact(ctx, db.GetCustomerContactParams{OrganizationID: orgID, CustomerID: customerID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) CreateContact(ctx context.Context, orgID, customerID int64, c db.CustomerContact) (db.CustomerContact, error) {
	row, err := r.q.CreateCustomerContact(ctx, db.CreateCustomerContactParams{
		OrganizationID: orgID, CustomerID: customerID, FullName: c.FullName, Title: c.Title, Phone: c.Phone,
		Email: c.Email, IsPrimary: c.IsPrimary, Note: c.Note,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateContact(ctx context.Context, orgID, customerID int64, c db.CustomerContact) (db.CustomerContact, error) {
	row, err := r.q.UpdateCustomerContact(ctx, db.UpdateCustomerContactParams{
		OrganizationID: orgID, CustomerID: customerID, Uuid: c.Uuid, FullName: c.FullName, Title: c.Title,
		Phone: c.Phone, Email: c.Email, IsPrimary: c.IsPrimary, Note: c.Note,
	})
	return row, mapErr(err)
}

func (r *Postgres) ClearPrimaryContact(ctx context.Context, orgID, customerID int64, keep uuid.UUID) error {
	return r.q.ClearPrimaryCustomerContact(ctx, db.ClearPrimaryCustomerContactParams{OrganizationID: orgID, CustomerID: customerID, KeepUuid: keep})
}

func (r *Postgres) DeleteContact(ctx context.Context, orgID, customerID int64, id uuid.UUID) error {
	n, err := r.q.DeleteCustomerContact(ctx, db.DeleteCustomerContactParams{OrganizationID: orgID, CustomerID: customerID, Uuid: id})
	return affected(n, err)
}

func (r *Postgres) DeleteContacts(ctx context.Context, orgID, customerID int64) error {
	return r.q.DeleteCustomerContacts(ctx, db.DeleteCustomerContactsParams{OrganizationID: orgID, CustomerID: customerID})
}

// --- consents -------------------------------------------------------------------

func (r *Postgres) ListConsents(ctx context.Context, orgID, customerID int64) ([]ConsentRow, error) {
	return r.q.ListCustomerConsents(ctx, db.ListCustomerConsentsParams{OrganizationID: orgID, CustomerID: customerID})
}

func (r *Postgres) GrantConsent(ctx context.Context, p db.GrantCustomerConsentParams) (db.CustomerMarketingConsent, error) {
	row, err := r.q.GrantCustomerConsent(ctx, p)
	return row, mapErr(err)
}

// WithdrawConsent closes the open grant of a channel; false when none is open.
func (r *Postgres) WithdrawConsent(ctx context.Context, p db.WithdrawCustomerConsentParams) (bool, error) {
	n, err := r.q.WithdrawCustomerConsent(ctx, p)
	return n > 0, err
}

func (r *Postgres) WithdrawAllConsents(ctx context.Context, p db.WithdrawAllCustomerConsentsParams) error {
	return r.q.WithdrawAllCustomerConsents(ctx, p)
}

// --- drivers --------------------------------------------------------------------

func (r *Postgres) ListDrivers(ctx context.Context, orgID, customerID int64) ([]db.Driver, error) {
	return r.q.ListCustomerDrivers(ctx, db.ListCustomerDriversParams{OrganizationID: orgID, CustomerID: customerID})
}

func (r *Postgres) GetDriver(ctx context.Context, orgID, customerID int64, id uuid.UUID) (db.Driver, error) {
	row, err := r.q.GetCustomerDriver(ctx, db.GetCustomerDriverParams{OrganizationID: orgID, CustomerID: customerID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetDriverByUUID(ctx context.Context, orgID int64, id uuid.UUID) (db.GetDriverByUUIDRow, error) {
	row, err := r.q.GetDriverByUUID(ctx, db.GetDriverByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) LicenceTaken(ctx context.Context, orgID, customerID int64, bidx string, exclude *uuid.UUID) (bool, error) {
	if bidx == "" {
		return false, nil
	}
	var ex pgtype.UUID
	if exclude != nil {
		ex = pgtype.UUID{Bytes: *exclude, Valid: true}
	}
	return r.q.DriverLicenceTaken(ctx, db.DriverLicenceTakenParams{
		OrganizationID: orgID, CustomerID: customerID, LicenceBidx: bidx, ExcludeUuid: ex,
	})
}

func (r *Postgres) CreateDriver(ctx context.Context, orgID, customerID int64, f DriverFields, actor pgtype.Int8) (db.Driver, error) {
	row, err := r.q.CreateDriver(ctx, db.CreateDriverParams{
		OrganizationID: orgID, CustomerID: customerID, IsCustomerSelf: f.IsCustomerSelf, FirstName: f.FirstName,
		LastName: f.LastName, BirthDate: f.BirthDate, Phone: f.Phone, Email: f.Email, IdentityType: f.IdentityType,
		IdentityEnc: f.IdentityEnc, IdentityBidx: f.IdentityBidx, IdentityHint: f.IdentityHint,
		Nationality: f.Nationality, LicenceEnc: f.LicenceEnc, LicenceBidx: f.LicenceBidx, LicenceHint: f.LicenceHint,
		LicenceClass: f.LicenceClass, LicenceIssuedOn: f.LicenceIssuedOn, LicenceExpiresOn: f.LicenceExpiresOn,
		LicenceCountry: f.LicenceCountry, Note: f.Note, CreatedBy: actor,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateDriver(ctx context.Context, orgID, customerID int64, id uuid.UUID, f DriverFields) (db.Driver, error) {
	row, err := r.q.UpdateDriver(ctx, db.UpdateDriverParams{
		OrganizationID: orgID, CustomerID: customerID, Uuid: id, IsCustomerSelf: f.IsCustomerSelf,
		FirstName: f.FirstName, LastName: f.LastName, BirthDate: f.BirthDate, Phone: f.Phone, Email: f.Email,
		IdentityType: f.IdentityType, IdentityEnc: f.IdentityEnc, IdentityBidx: f.IdentityBidx,
		IdentityHint: f.IdentityHint, Nationality: f.Nationality, LicenceEnc: f.LicenceEnc,
		LicenceBidx: f.LicenceBidx, LicenceHint: f.LicenceHint, LicenceClass: f.LicenceClass,
		LicenceIssuedOn: f.LicenceIssuedOn, LicenceExpiresOn: f.LicenceExpiresOn,
		LicenceCountry: f.LicenceCountry, Note: f.Note,
	})
	return row, mapErr(err)
}

func (r *Postgres) DeleteDriver(ctx context.Context, orgID, customerID int64, id uuid.UUID) error {
	n, err := r.q.DeleteDriver(ctx, db.DeleteDriverParams{OrganizationID: orgID, CustomerID: customerID, Uuid: id})
	return affected(n, err)
}

func (r *Postgres) AnonymizeDrivers(ctx context.Context, orgID, customerID int64) error {
	return r.q.AnonymizeCustomerDrivers(ctx, db.AnonymizeCustomerDriversParams{OrganizationID: orgID, CustomerID: customerID})
}

// --- documents ------------------------------------------------------------------

func (r *Postgres) ListDocuments(ctx context.Context, orgID, customerID int64) ([]DocumentRow, error) {
	return r.q.ListCustomerDocuments(ctx, db.ListCustomerDocumentsParams{OrganizationID: orgID, CustomerID: customerID})
}

func (r *Postgres) GetDocument(ctx context.Context, orgID, customerID int64, id uuid.UUID) (DocumentRow, error) {
	row, err := r.q.GetCustomerDocument(ctx, db.GetCustomerDocumentParams{OrganizationID: orgID, CustomerID: customerID, Uuid: id})
	return DocumentRow(row), mapErr(err)
}

func (r *Postgres) CreateDocument(ctx context.Context, p db.CreateCustomerDocumentParams) (db.CustomerDocument, error) {
	row, err := r.q.CreateCustomerDocument(ctx, p)
	return row, mapErr(err)
}

func (r *Postgres) SetDocumentFile(ctx context.Context, p db.SetCustomerDocumentFileParams) (db.CustomerDocument, error) {
	row, err := r.q.SetCustomerDocumentFile(ctx, p)
	return row, mapErr(err)
}

func (r *Postgres) DeleteDocument(ctx context.Context, orgID, customerID int64, id uuid.UUID) error {
	n, err := r.q.DeleteCustomerDocument(ctx, db.DeleteCustomerDocumentParams{OrganizationID: orgID, CustomerID: customerID, Uuid: id})
	return affected(n, err)
}

func (r *Postgres) DocumentKeys(ctx context.Context, orgID, customerID int64) ([]string, error) {
	return r.q.ListCustomerDocumentKeys(ctx, db.ListCustomerDocumentKeysParams{OrganizationID: orgID, CustomerID: customerID})
}

func (r *Postgres) PurgeDocuments(ctx context.Context, orgID, customerID int64) error {
	return r.q.PurgeCustomerDocuments(ctx, db.PurgeCustomerDocumentsParams{OrganizationID: orgID, CustomerID: customerID})
}

// --- shared ---------------------------------------------------------------------

func (r *Postgres) OrganizationUUID(ctx context.Context, orgID int64) (uuid.UUID, error) {
	org, err := r.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return uuid.Nil, mapErr(err)
	}
	return org.Uuid, nil
}

func (r *Postgres) ListActivity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]db.ListOrganizationResourceActivityRow, int64, error) {
	ref := pgtype.UUID{Bytes: id, Valid: true}
	rows, err := r.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Resource, ResourceUuid: ref,
		LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Resource, ResourceUuid: ref,
	})
	return rows, total, err
}

// Queries exposes the generated queries (search adapter, organization walk).
func (r *Postgres) Queries() *db.Queries { return r.q }

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "uq_customers_organization_identity":
			return ErrIdentityTaken
		case "uq_drivers_organization_customer_licence":
			return ErrLicenceTaken
		case "uq_customer_marketing_consents_open":
			return ErrConsentOpen
		}
	}
	return err
}

func affected(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func stringsArg(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func tagsArg(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
