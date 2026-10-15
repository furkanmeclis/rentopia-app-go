package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Driver age bounds (plausibility of the birth date).
const (
	minDriverAgeYears = 16
	maxDriverAgeYears = 110
)

// DriverChange is the outcome of a driver mutation (activity / live events).
type DriverChange struct {
	CustomerUUID uuid.UUID
	Driver       model.Driver
}

// ListDrivers returns the drivers of a customer (masked).
func (s *Service) ListDrivers(ctx context.Context, orgID int64, customerUUID uuid.UUID) ([]model.Driver, error) {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListDrivers(ctx, orgID, c.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Driver, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.mapDriver(row, c.Uuid))
	}
	return out, nil
}

// GetDriver returns one driver of a customer (masked).
func (s *Service) GetDriver(ctx context.Context, orgID int64, customerUUID, id uuid.UUID) (model.Driver, error) {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return model.Driver{}, err
	}
	row, err := s.repo.GetDriver(ctx, orgID, c.ID, id)
	if err != nil {
		return model.Driver{}, mapNotFound(err, ErrDriverNotFound)
	}
	return s.mapDriver(row, c.Uuid), nil
}

// CreateDriver adds a driver to a customer. CopyFromCustomer copies the
// person of an individual customer (name, identity, nationality, phone,
// e-mail) and marks the driver as the customer; the licence comes from the
// input.
func (s *Service) CreateDriver(ctx context.Context, orgID int64, customerUUID uuid.UUID, in model.DriverInput) (DriverChange, error) {
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return DriverChange{}, err
	}
	f := repository.DriverFields{}
	if in.CopyFromCustomer {
		if c.CustomerType != model.TypeIndividual {
			return DriverChange{}, validation("copy_from_customer", "not_individual", "only an individual customer can be its own driver")
		}
		if err := s.copyCustomerPerson(orgID, c, &f); err != nil {
			return DriverChange{}, err
		}
	}
	if err := s.applyDriver(orgID, &f, in, true); err != nil {
		return DriverChange{}, err
	}
	if taken, err := s.repo.LicenceTaken(ctx, orgID, c.ID, f.LicenceBidx, nil); err != nil {
		return DriverChange{}, err
	} else if taken {
		return DriverChange{}, ErrLicenceTaken
	}
	row, err := s.repo.CreateDriver(ctx, orgID, c.ID, f, actorArg(ctx))
	if err != nil {
		return DriverChange{}, mapErr(err)
	}
	return DriverChange{CustomerUUID: c.Uuid, Driver: s.mapDriver(row, c.Uuid)}, nil
}

// PatchDriver updates the given fields of a driver.
func (s *Service) PatchDriver(ctx context.Context, orgID int64, customerUUID, id uuid.UUID, in model.DriverInput) (DriverChange, error) {
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return DriverChange{}, err
	}
	current, err := s.repo.GetDriver(ctx, orgID, c.ID, id)
	if err != nil {
		return DriverChange{}, mapNotFound(err, ErrDriverNotFound)
	}
	f := repository.FieldsFromDriver(current)
	if err := s.applyDriver(orgID, &f, in, false); err != nil {
		return DriverChange{}, err
	}
	if f.LicenceBidx != current.LicenceBidx {
		if taken, err := s.repo.LicenceTaken(ctx, orgID, c.ID, f.LicenceBidx, &id); err != nil {
			return DriverChange{}, err
		} else if taken {
			return DriverChange{}, ErrLicenceTaken
		}
	}
	row, err := s.repo.UpdateDriver(ctx, orgID, c.ID, id, f)
	if err != nil {
		return DriverChange{}, mapNotFound(err, ErrDriverNotFound)
	}
	return DriverChange{CustomerUUID: c.Uuid, Driver: s.mapDriver(row, c.Uuid)}, nil
}

// DeleteDriver soft-deletes a driver.
func (s *Service) DeleteDriver(ctx context.Context, orgID int64, customerUUID, id uuid.UUID) error {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteDriver(ctx, orgID, c.ID, id); err != nil {
		return mapNotFound(err, ErrDriverNotFound)
	}
	return nil
}

// DriverEligibility checks a driver of a customer against rental
// conditions at a time (reservations: the product version's conditions and
// the pickup time).
func (s *Service) DriverEligibility(ctx context.Context, orgID int64, customerUUID, id uuid.UUID, c model.Conditions, at time.Time) (model.Eligibility, error) {
	cust, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return model.Eligibility{}, err
	}
	row, err := s.repo.GetDriver(ctx, orgID, cust.ID, id)
	if err != nil {
		return model.Eligibility{}, mapNotFound(err, ErrDriverNotFound)
	}
	return model.EligibleForConditions(DriverFacts(row), c, at), nil
}

// EligibleDriver resolves any driver of the organization by uuid and checks
// it (the reservations entry point: a driver_uuid from a reservation body;
// another organization's uuid is ErrDriverNotFound).
func (s *Service) EligibleDriver(ctx context.Context, orgID int64, driverUUID uuid.UUID, c model.Conditions, at time.Time) (model.Eligibility, uuid.UUID, error) {
	row, err := s.repo.GetDriverByUUID(ctx, orgID, driverUUID)
	if err != nil {
		return model.Eligibility{}, uuid.Nil, mapNotFound(err, ErrDriverNotFound)
	}
	d := db.Driver{
		BirthDate: row.BirthDate, LicenceEnc: row.LicenceEnc, LicenceIssuedOn: row.LicenceIssuedOn,
		LicenceExpiresOn: row.LicenceExpiresOn,
	}
	return model.EligibleForConditions(DriverFacts(d), c, at), row.CustomerUuid, nil
}

// DriverFacts extracts the plain fields of the condition check.
func DriverFacts(d db.Driver) model.DriverFacts {
	out := model.DriverFacts{HasLicence: d.LicenceEnc != ""}
	if d.BirthDate.Valid {
		t := d.BirthDate.Time
		out.BirthDate = &t
	}
	if d.LicenceIssuedOn.Valid {
		t := d.LicenceIssuedOn.Time
		out.LicenceIssuedOn = &t
	}
	if d.LicenceExpiresOn.Valid {
		t := d.LicenceExpiresOn.Time
		out.LicenceExpiresOn = &t
	}
	return out
}

// copyCustomerPerson fills a driver from an individual customer; the
// identity is re-encrypted for the driver column.
func (s *Service) copyCustomerPerson(orgID int64, c db.Customer, f *repository.DriverFields) error {
	f.IsCustomerSelf = true
	f.FirstName, f.LastName = c.FirstName, c.LastName
	f.Phone, f.Email = c.Phone, c.Email
	f.Nationality = c.Nationality
	if c.IdentityEnc == "" || !c.IdentityType.Valid {
		return nil
	}
	plain, err := s.open(orgID, colCustomerIdentity, c.IdentityEnc)
	if err != nil {
		return err
	}
	sv, err := s.seal(orgID, colDriverIdentity, bidxDriverIdentity, c.IdentityType.String, plain)
	if err != nil {
		return err
	}
	f.IdentityType = c.IdentityType
	f.IdentityEnc, f.IdentityBidx, f.IdentityHint = sv.enc, sv.bidx, sv.hint
	return nil
}

func (s *Service) applyDriver(orgID int64, f *repository.DriverFields, in model.DriverInput, create bool) error {
	ve := &ValidationError{}
	applyText(ve, "first_name", in.FirstName, 128, &f.FirstName)
	applyText(ve, "last_name", in.LastName, 128, &f.LastName)
	if f.FirstName == "" {
		ve.add("first_name", "required", "first_name is required")
	}
	if f.LastName == "" {
		ve.add("last_name", "required", "last_name is required")
	}
	applyDate(ve, "birth_date", in.BirthDate, &f.BirthDate)
	if f.BirthDate.Valid && in.BirthDate.Set {
		age := model.FullYears(f.BirthDate.Time, s.now())
		if f.BirthDate.Time.After(s.now()) || age < minDriverAgeYears || age > maxDriverAgeYears {
			ve.add("birth_date", "out_of_range", "birth_date is not plausible for a driver")
		}
	}
	applyPhone(ve, "phone", in.Phone, &f.Phone)
	applyEmail(ve, "email", in.Email, &f.Email)

	kind := ""
	if f.IdentityType.Valid {
		kind = f.IdentityType.String
	}
	if in.IdentityType.Set {
		kind = ""
		if in.IdentityType.Value != nil {
			kind = strings.TrimSpace(*in.IdentityType.Value)
		}
	}
	switch {
	case in.IdentityNumber != nil && strings.TrimSpace(*in.IdentityNumber) == "":
		f.IdentityType, f.IdentityEnc, f.IdentityBidx, f.IdentityHint = pgtype.Text{}, "", "", ""
	case in.IdentityNumber != nil:
		if kind != model.IdentityTCKN && kind != model.IdentityPassport {
			ve.add("identity_type", "invalid", "identity_type must be tckn or passport")
			break
		}
		norm, d := normalizeIdentity("identity_number", kind, *in.IdentityNumber)
		if d != nil {
			ve.Details = append(ve.Details, *d)
			break
		}
		sv, err := s.seal(orgID, colDriverIdentity, bidxDriverIdentity, kind, norm)
		if err != nil {
			return err
		}
		f.IdentityType = pgtype.Text{String: kind, Valid: true}
		f.IdentityEnc, f.IdentityBidx, f.IdentityHint = sv.enc, sv.bidx, sv.hint
	case in.IdentityType.Set && kind != f.IdentityType.String:
		if kind == "" {
			f.IdentityType, f.IdentityEnc, f.IdentityBidx, f.IdentityHint = pgtype.Text{}, "", "", ""
		} else {
			ve.add("identity_number", "required", "identity_number is required when identity_type changes")
		}
	}
	applyCountry(ve, "nationality", in.Nationality, &f.Nationality)

	if in.LicenceNumber != nil {
		raw := strings.TrimSpace(*in.LicenceNumber)
		if raw == "" {
			f.LicenceEnc, f.LicenceBidx, f.LicenceHint = "", "", ""
		} else {
			doc := pii.Document(raw)
			if len(doc) < 4 || len(doc) > 32 {
				ve.add("licence_number", "invalid", "licence_number must have 4 to 32 letters or digits")
			} else {
				sv, err := s.seal(orgID, colDriverLicence, bidxDriverLicence, "licence", doc)
				if err != nil {
					return err
				}
				f.LicenceEnc, f.LicenceBidx, f.LicenceHint = sv.enc, sv.bidx, sv.hint
			}
		}
	}
	if in.LicenceClass != nil {
		applyText(ve, "licence_class", model.Str(strings.ToUpper(strings.TrimSpace(*in.LicenceClass))), 32, &f.LicenceClass)
	}
	applyDate(ve, "licence_issued_on", in.LicenceIssuedOn, &f.LicenceIssuedOn)
	applyDate(ve, "licence_expires_on", in.LicenceExpiresOn, &f.LicenceExpiresOn)
	if f.LicenceIssuedOn.Valid && f.LicenceIssuedOn.Time.After(s.now()) {
		ve.add("licence_issued_on", "in_future", "licence_issued_on must not be in the future")
	}
	if f.LicenceIssuedOn.Valid && f.LicenceExpiresOn.Valid && f.LicenceExpiresOn.Time.Before(f.LicenceIssuedOn.Time) {
		ve.add("licence_expires_on", "before_issued_on", "licence_expires_on must not be before licence_issued_on")
	}
	if f.BirthDate.Valid && f.LicenceIssuedOn.Valid && f.LicenceIssuedOn.Time.Before(f.BirthDate.Time) {
		ve.add("licence_issued_on", "before_birth_date", "licence_issued_on must be after birth_date")
	}
	applyCountry(ve, "licence_country", in.LicenceCountry, &f.LicenceCountry)
	applyText(ve, "note", in.Note, 2000, &f.Note)
	_ = create
	return ve.orNil()
}

func (s *Service) mapDriver(d db.Driver, customerUUID uuid.UUID) model.Driver {
	now := s.now()
	out := model.Driver{
		UUID: d.Uuid, CustomerUUID: customerUUID, IsCustomerSelf: d.IsCustomerSelf, FirstName: d.FirstName,
		LastName: d.LastName, FullName: strings.TrimSpace(d.FirstName + " " + d.LastName),
		BirthDate: datePtr(d.BirthDate), PhoneMasked: pii.MaskPhone(d.Phone), EmailMasked: pii.MaskEmail(d.Email),
		IdentityType: textPtr(d.IdentityType), IdentityMasked: d.IdentityHint, Nationality: textPtr(d.Nationality),
		LicenceMasked: d.LicenceHint, LicenceClass: d.LicenceClass, LicenceIssuedOn: datePtr(d.LicenceIssuedOn),
		LicenceExpiresOn: datePtr(d.LicenceExpiresOn), LicenceCountry: textPtr(d.LicenceCountry),
		ForeignLicence: d.LicenceCountry.Valid && d.LicenceCountry.String != "TR",
		Note:           d.Note, CreatedAt: d.CreatedAt.Time.UTC(), UpdatedAt: d.UpdatedAt.Time.UTC(),
	}
	facts := model.EligibleForConditions(DriverFacts(d), model.Conditions{}, now)
	out.Age, out.LicenceYears = facts.Age, facts.LicenceYears
	for _, r := range facts.Reasons {
		if r == model.ReasonLicenceExpired {
			out.LicenceExpired = true
		}
	}
	return out
}
