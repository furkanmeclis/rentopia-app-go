package usecase

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
)

// Reveal reason bounds.
const (
	MinReasonLength = 3
	MaxReasonLength = 500
)

// CheckReason validates the reason of a reveal / anonymization (the
// customer is resolved first, so another organization's uuid answers 404
// before any validation).
func CheckReason(reason string) (string, error) {
	r := strings.TrimSpace(reason)
	n := utf8.RuneCountInString(r)
	switch {
	case n == 0:
		return "", validation("reason", "required", "reason is required")
	case n < MinReasonLength:
		return "", validation("reason", "too_short", "reason must be at least 3 characters")
	case n > MaxReasonLength:
		return "", validation("reason", "too_long", "reason must be at most 500 characters")
	}
	return r, nil
}

// Reveal decrypts the sensitive values of a customer, its contacts and its
// drivers. The caller (handler) must hold tenant.customers.reveal and
// record the activity event with the reason; the values are never logged.
func (s *Service) Reveal(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Revealed, error) {
	c, err := s.getCustomer(ctx, orgID, id)
	if err != nil {
		return model.Revealed{}, err
	}
	if _, err := CheckReason(reason); err != nil {
		return model.Revealed{}, err
	}
	identity, err := s.open(orgID, colCustomerIdentity, c.IdentityEnc)
	if err != nil {
		return model.Revealed{}, err
	}
	out := model.Revealed{
		CustomerUUID: c.Uuid, IdentityType: textPtr(c.IdentityType), IdentityNumber: identity,
		Phone: c.Phone, Email: c.Email, Contacts: []model.RevealedContact{}, Drivers: []model.RevealedDriver{},
		RevealedAt: s.now().UTC(), DisplaySeconds: model.RevealDisplaySeconds,
	}
	contacts, err := s.repo.ListContacts(ctx, orgID, c.ID)
	if err != nil {
		return model.Revealed{}, err
	}
	for _, ct := range contacts {
		out.Contacts = append(out.Contacts, model.RevealedContact{UUID: ct.Uuid, Phone: ct.Phone, Email: ct.Email})
	}
	drivers, err := s.repo.ListDrivers(ctx, orgID, c.ID)
	if err != nil {
		return model.Revealed{}, err
	}
	for _, d := range drivers {
		rd, err := s.revealDriver(orgID, d)
		if err != nil {
			return model.Revealed{}, err
		}
		out.Drivers = append(out.Drivers, rd)
	}
	return out, nil
}

func (s *Service) revealDriver(orgID int64, d db.Driver) (model.RevealedDriver, error) {
	identity, err := s.open(orgID, colDriverIdentity, d.IdentityEnc)
	if err != nil {
		return model.RevealedDriver{}, err
	}
	licence, err := s.open(orgID, colDriverLicence, d.LicenceEnc)
	if err != nil {
		return model.RevealedDriver{}, err
	}
	return model.RevealedDriver{
		UUID: d.Uuid, IdentityType: textPtr(d.IdentityType), IdentityNumber: identity, LicenceNumber: licence,
		Phone: d.Phone, Email: d.Email,
	}, nil
}

// Anonymization is the outcome of Anonymize (for the activity payload).
type Anonymization struct {
	CustomerUUID uuid.UUID
	Drivers      int
	Documents    int
	Contacts     int
}

// Anonymize wipes the personal data of a customer for a data subject
// request (KVKK art. 7 / 11): names, identity, contact, billing, note and
// tags of the customer; its contacts are deleted, its drivers wiped and
// removed, its document files deleted from storage and their rows purged,
// open marketing consents withdrawn (the consent history stays as proof).
// The customer row itself stays (reservations and invoices reference it)
// and reads as "anonymized". It cannot be undone.
func (s *Service) Anonymize(ctx context.Context, orgID int64, id uuid.UUID, reason string) (Anonymization, error) {
	c, err := s.getCustomer(ctx, orgID, id)
	if err != nil {
		return Anonymization{}, err
	}
	if _, err := CheckReason(reason); err != nil {
		return Anonymization{}, err
	}
	if c.AnonymizedAt.Valid {
		return Anonymization{}, ErrAnonymized
	}
	out := Anonymization{CustomerUUID: c.Uuid}
	keys, err := s.repo.DocumentKeys(ctx, orgID, c.ID)
	if err != nil {
		return Anonymization{}, err
	}
	contacts, err := s.repo.ListContacts(ctx, orgID, c.ID)
	if err != nil {
		return Anonymization{}, err
	}
	drivers, err := s.repo.ListDrivers(ctx, orgID, c.ID)
	if err != nil {
		return Anonymization{}, err
	}
	out.Documents, out.Contacts, out.Drivers = len(keys), len(contacts), len(drivers)
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if err := tx.PurgeDocuments(ctx, orgID, c.ID); err != nil {
			return err
		}
		if err := tx.DeleteContacts(ctx, orgID, c.ID); err != nil {
			return err
		}
		if err := tx.AnonymizeDrivers(ctx, orgID, c.ID); err != nil {
			return err
		}
		if err := tx.WithdrawAllConsents(ctx, db.WithdrawAllCustomerConsentsParams{
			OrganizationID: orgID, CustomerID: c.ID, WithdrawnAt: tstz(s.now()),
		}); err != nil {
			return err
		}
		return tx.AnonymizeCustomer(ctx, orgID, c.ID)
	})
	if err != nil {
		return Anonymization{}, mapErr(err)
	}
	// Files go after the rows are gone (a failed delete leaves an orphan
	// object under the organization prefix, never a row pointing at data).
	if s.storage != nil {
		orgUUID, err := s.repo.OrganizationUUID(ctx, orgID)
		if err == nil {
			for _, key := range keys {
				if !storage.OrganizationOwnsKey(key, orgUUID.String()) {
					continue
				}
				if err := s.storage.Delete(ctx, key); err != nil {
					s.log.Warn("customer_document_delete_failed", "customer", c.Uuid.String(), "error", err)
				}
			}
		}
	}
	s.index(ctx, orgID, c.Uuid)
	return out, nil
}
