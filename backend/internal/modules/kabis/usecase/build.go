package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pii"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// built is a notification payload with its validation.
type built struct {
	n          kabis.Notification
	missing    []string
	summary    map[string]any
	branchID   int64
	rentalBr   int64
	occurredAt time.Time
	plate      string
}

// errNotHandedOver: the rental has no actual event of that kind yet.
var errNotHandedOver = errors.New("kabis: the rental has no actual event to report")

// build makes the payload of an event from the rental's ACTUAL facts and
// the current person / vehicle records (never from a plan). Missing
// required data is listed by field (dot paths), never guessed.
func (s *Service) build(ctx context.Context, q *db.Queries, orgID, rentalID int64, event string, ref uuid.UUID, corrects string) (built, error) {
	f, err := q.GetKabisRentalFacts(ctx, db.GetKabisRentalFactsParams{OrganizationID: orgID, ID: rentalID})
	if err != nil {
		return built{}, err
	}
	b := built{rentalBr: f.RentalBranchID, plate: strings.TrimSpace(f.VehiclePlate)}
	var branch, point pgtype.Int8
	switch event {
	case kabis.EventRentalEnd:
		if !f.ActualReturnAt.Valid {
			return built{}, errNotHandedOver
		}
		b.occurredAt, branch, point = f.ActualReturnAt.Time.UTC(), f.ActualReturnBranchID, f.ActualReturnPointID
	default: // start and its corrections: the actual handover
		if !f.ActualPickupAt.Valid {
			return built{}, errNotHandedOver
		}
		b.occurredAt, branch, point = f.ActualPickupAt.Time.UTC(), f.ActualPickupBranchID, f.ActualPickupPointID
	}
	b.branchID = f.RentalBranchID
	if branch.Valid {
		b.branchID = branch.Int64
	}
	n := kabis.Notification{
		Reference: ref.String(), Event: event, Corrects: corrects, RentalReference: f.Reference, OccurredAt: b.occurredAt,
		Vehicle: kabis.Vehicle{Plate: b.plate, BrandName: strings.TrimSpace(f.VehicleLabel)},
	}
	miss := func(field string) { b.missing = append(b.missing, field) }
	if n.Vehicle.Plate == "" {
		miss("vehicle.plate")
	}
	place, err := q.GetKabisPlace(ctx, db.GetKabisPlaceParams{OrganizationID: orgID, ID: b.branchID, PointID: point})
	if err != nil {
		return built{}, err
	}
	n.Place = kabis.Place{
		BranchCode: place.Code, Name: strings.TrimSpace(place.Name), City: strings.TrimSpace(place.City),
		District: strings.TrimSpace(place.District), Address: strings.TrimSpace(place.Address), Point: place.PointName,
	}
	if n.Place.Name == "" {
		miss("place.name")
	}
	if n.Place.City == "" {
		miss("place.city")
	}

	// Renter: the reservation's customer.
	c, err := q.GetKabisCustomer(ctx, db.GetKabisCustomerParams{OrganizationID: orgID, ID: f.CustomerID})
	if err != nil {
		return built{}, err
	}
	renter := kabis.Person{Role: "renter", Type: c.CustomerType, Nationality: strings.TrimSpace(c.Nationality.String)}
	if c.CustomerType == "corporate" {
		renter.CompanyTitle = strings.TrimSpace(c.CompanyTitle)
		if renter.CompanyTitle == "" {
			miss("renter.company_title")
		}
	} else {
		renter.FirstName, renter.LastName = strings.TrimSpace(c.FirstName), strings.TrimSpace(c.LastName)
		if renter.FirstName == "" {
			miss("renter.first_name")
		}
		if renter.LastName == "" {
			miss("renter.last_name")
		}
	}
	renter.IdentityType = c.IdentityType.String
	renter.IdentityNumber = s.open(orgID, customersusecase.ColumnCustomerIdentity, c.IdentityEnc)
	if renter.IdentityNumber == "" || renter.IdentityType == "" {
		miss("renter.identity_number")
	}
	if renter.IdentityType == pii.KindPassport && renter.Nationality == "" {
		miss("renter.nationality")
	}
	n.Renter = renter

	// Drivers: every driver of the rental.
	drivers, err := q.ListKabisRentalDrivers(ctx, db.ListKabisRentalDriversParams{OrganizationID: orgID, RentalID: rentalID})
	if err != nil {
		return built{}, err
	}
	if len(drivers) == 0 {
		miss("drivers")
	}
	for i, d := range drivers {
		p := "drivers." + strconv.Itoa(i) + "."
		person := kabis.Person{
			Role: d.Role, FirstName: strings.TrimSpace(d.FirstName), LastName: strings.TrimSpace(d.LastName),
			IdentityType: d.IdentityType.String, Nationality: strings.TrimSpace(d.Nationality.String),
			LicenceClass: d.LicenceClass, LicenceCountry: strings.TrimSpace(d.LicenceCountry.String),
		}
		if d.BirthDate.Valid {
			person.BirthDate = d.BirthDate.Time.Format("2006-01-02")
		}
		if !d.HasRecord {
			miss(p + "record")
			n.Drivers = append(n.Drivers, person)
			continue
		}
		person.IdentityNumber = s.open(orgID, customersusecase.ColumnDriverIdentity, d.IdentityEnc)
		person.LicenceNumber = s.open(orgID, customersusecase.ColumnDriverLicence, d.LicenceEnc)
		if person.FirstName == "" {
			miss(p + "first_name")
		}
		if person.LastName == "" {
			miss(p + "last_name")
		}
		if person.IdentityNumber == "" || person.IdentityType == "" {
			miss(p + "identity_number")
		}
		if person.IdentityType == pii.KindPassport && person.Nationality == "" {
			miss(p + "nationality")
		}
		if person.LicenceNumber == "" {
			miss(p + "licence_number")
		}
		n.Drivers = append(n.Drivers, person)
	}
	b.n = n
	b.summary = summaryOf(n)
	return b, nil
}

// open decrypts a sealed customer / driver value; a value that cannot be
// opened counts as missing (logged without the value).
func (s *Service) open(orgID int64, column, enc string) string {
	v, err := customersusecase.OpenSensitive(s.cipher, orgID, column, enc)
	if err != nil {
		s.log.Warn("kabis_open_sensitive_failed", "organization_id", orgID, "column", column, "error", err)
		return ""
	}
	return strings.TrimSpace(v)
}

// summaryOf is the masked display form of a payload.
func summaryOf(n kabis.Notification) map[string]any {
	person := func(p kabis.Person) map[string]any {
		idKind := p.IdentityType
		out := map[string]any{
			"role": p.Role, "type": p.Type, "first_name": p.FirstName, "last_name": p.LastName,
			"company_title": p.CompanyTitle, "identity_type": p.IdentityType,
			"identity_hint": pii.MaskIdentity(idKind, p.IdentityNumber), "nationality": p.Nationality,
			"licence_hint": pii.MaskIdentity(pii.KindLicence, p.LicenceNumber), "licence_class": p.LicenceClass,
		}
		return out
	}
	drivers := make([]map[string]any, 0, len(n.Drivers))
	for _, d := range n.Drivers {
		drivers = append(drivers, person(d))
	}
	return map[string]any{
		"event": n.Event, "rental_reference": n.RentalReference, "occurred_at": n.OccurredAt,
		"place": map[string]any{
			"branch_code": n.Place.BranchCode, "name": n.Place.Name, "city": n.Place.City, "district": n.Place.District,
			"address": n.Place.Address, "point": n.Place.Point,
		},
		"vehicle": map[string]any{"plate": n.Vehicle.Plate, "brand_model": n.Vehicle.BrandName},
		"renter":  person(n.Renter), "drivers": drivers,
	}
}

// contentHash hashes the reported facts (not our own references), so a
// correction is made only when the facts change.
func contentHash(n kabis.Notification) string {
	n.Reference, n.Event, n.Corrects = "", "", ""
	raw, _ := json.Marshal(n)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func payloadAAD(orgID int64, id uuid.UUID) string {
	return "kabis_notifications.payload|org:" + strconv.FormatInt(orgID, 10) + "|notification:" + id.String()
}

func (s *Service) seal(orgID int64, id uuid.UUID, n kabis.Notification) (string, error) {
	raw, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return s.sealer.Seal(string(raw), payloadAAD(orgID, id))
}

func (s *Service) unseal(orgID int64, id uuid.UUID, enc string) (kabis.Notification, error) {
	var n kabis.Notification
	plain, err := s.sealer.Open(enc, payloadAAD(orgID, id))
	if err != nil {
		return n, fmt.Errorf("kabis: open payload: %w", err)
	}
	err = json.Unmarshal([]byte(plain), &n)
	return n, err
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
