package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// renewGraceHours keeps the previous site key working for the booking
// sessions visitors already hold (30 minutes) after "renew key".
const renewGraceHours = 1

func requireVersion(in model.VersionInput) (int, error) {
	if in.Version == nil || *in.Version < 0 {
		ve := &ValidationError{}
		ve.add("version", "required", "send the version you edited")
		return 0, ve
	}
	return *in.Version, nil
}

func actor(userID int64) pgtype.Int8 { return pgtype.Int8{Int64: userID, Valid: userID > 0} }

// ensureRow returns the settings row, storing the defaults first when the
// caller edits a site that was never saved (version 0).
func (s *Service) ensureRow(ctx context.Context, orgID int64, orgSlug string, userID int64, version int) (db.GetSiteSettingsRow, int, error) {
	row, err := s.q.GetSiteSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		if version != 0 {
			return db.GetSiteSettingsRow{}, 0, ErrVersionConflict
		}
		if _, err := s.Save(ctx, orgID, orgSlug, userID, model.SaveInput{Version: ptrInt(0)}); err != nil {
			return db.GetSiteSettingsRow{}, 0, err
		}
		row, err = s.q.GetSiteSettings(ctx, orgID)
		if err != nil {
			return db.GetSiteSettingsRow{}, 0, err
		}
		return row, int(row.Version), nil
	}
	if err != nil {
		return db.GetSiteSettingsRow{}, 0, err
	}
	if int(row.Version) != version {
		return db.GetSiteSettingsRow{}, 0, ErrVersionConflict
	}
	return row, version, nil
}

// Publish makes the draft the caller saw (in.Version) the live site: a new
// revision in the history. A site that was never saved publishes its
// defaults. Publishing an unchanged draft is ErrNothingToPublish.
func (s *Service) Publish(ctx context.Context, orgID int64, orgSlug string, userID int64, in model.VersionInput) (model.Settings, int, error) {
	version, err := requireVersion(in)
	if err != nil {
		return model.Settings{}, 0, err
	}
	if _, version, err = s.ensureRow(ctx, orgID, orgSlug, userID, version); err != nil {
		return model.Settings{}, 0, err
	}
	cur, err := s.Get(ctx, orgID, orgSlug)
	if err != nil {
		return model.Settings{}, 0, err
	}
	if !cur.HasUnpublishedChanges {
		return model.Settings{}, 0, ErrNothingToPublish
	}
	pub, err := s.q.PublishSiteDraft(ctx, db.PublishSiteDraftParams{
		OrganizationID: orgID, ExpectedVersion: int32(version), PublishedBy: actor(userID),
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows), isUnique(err, "uq_site_setting_versions_revision"):
		return model.Settings{}, 0, ErrVersionConflict
	case err != nil:
		return model.Settings{}, 0, err
	}
	out, err := s.Get(ctx, orgID, orgSlug)
	return out, int(pub.Revision), err
}

// Versions lists the published revisions, newest first.
func (s *Service) Versions(ctx context.Context, orgID int64) ([]model.Revision, error) {
	current := int32(0)
	row, err := s.q.GetSiteSettings(ctx, orgID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return []model.Revision{}, nil
	case err != nil:
		return nil, err
	}
	if row.PublishedRevision.Valid {
		current = row.PublishedRevision.Int32
	}
	rows, err := s.q.ListSiteVersions(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Revision, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Revision{
			UUID: r.Uuid, Revision: int(r.Revision), DraftVersion: int(r.DraftVersion), PublishedAt: r.PublishedAt.Time.UTC(),
			PublishedBy: r.PublishedByName, Current: r.Revision == current,
			Assets: model.AssetState{Logo: r.HasLogo, Favicon: r.HasFavicon, Cover: r.HasCover},
		})
	}
	return out, nil
}

// Restore copies a published revision back into the draft (publish to make
// it live again). The revision is looked up first: another organization's
// revision is ErrRevisionNotFound whatever the body says.
func (s *Service) Restore(ctx context.Context, orgID int64, orgSlug string, userID int64, revision uuid.UUID,
	in model.VersionInput) (model.Settings, int, error) {
	rev, err := s.q.GetSiteVersion(ctx, db.GetSiteVersionParams{OrganizationID: orgID, Uuid: revision})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Settings{}, 0, ErrRevisionNotFound
	}
	if err != nil {
		return model.Settings{}, 0, err
	}
	version, err := requireVersion(in)
	if err != nil {
		return model.Settings{}, 0, err
	}
	_, err = s.q.RestoreSiteDraft(ctx, db.RestoreSiteDraftParams{
		OrganizationID: orgID, ExpectedVersion: int32(version), Uuid: revision, UpdatedBy: actor(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Settings{}, 0, ErrVersionConflict
	}
	if err != nil {
		return model.Settings{}, 0, err
	}
	out, err := s.Get(ctx, orgID, orgSlug)
	return out, int(rev.Revision), err
}

// RenewKey links a fresh publishable key to the site. A usable key is
// rotated (the new key keeps its origins and scopes, rotated_from_id
// points at the old one, which keeps working for an hour for open booking
// sessions); a missing, revoked or expired key is replaced by a new one.
func (s *Service) RenewKey(ctx context.Context, orgID int64, orgSlug string, userID int64, in model.VersionInput) (model.Settings, error) {
	version, err := requireVersion(in)
	if err != nil {
		return model.Settings{}, err
	}
	row, version, err := s.ensureRow(ctx, orgID, orgSlug, userID, version)
	if err != nil {
		return model.Settings{}, err
	}
	hosts, err := s.siteHosts(ctx, orgID, row.Subdomain)
	if err != nil {
		return model.Settings{}, err
	}
	var keyID pgtype.Int8
	usable := false
	if row.ApiKeyUuid.Valid {
		k, err := s.keys.GetKey(ctx, orgID, uuid.UUID(row.ApiKeyUuid.Bytes))
		usable = err == nil && k.Status == pamodel.KeyActive && (k.ExpiresAt == nil || k.ExpiresAt.After(s.now()))
	}
	if usable {
		grace := renewGraceHours
		issued, err := s.keys.RotateKey(ctx, orgID, uuid.UUID(row.ApiKeyUuid.Bytes), pamodel.RotateInput{GraceHours: &grace})
		if err != nil {
			return model.Settings{}, err
		}
		// Rotation copies the origins; ensureKey re-checks them.
		keyID, err = s.ensureKey(ctx, orgID, pgtype.UUID{Bytes: issued.Key.UUID, Valid: true}, nil, origins(hosts), true)
		if err != nil {
			return model.Settings{}, err
		}
	} else {
		keyID, err = s.ensureKey(ctx, orgID, pgtype.UUID{}, nil, origins(hosts), true)
		if err != nil {
			return model.Settings{}, err
		}
	}
	_, err = s.q.SetSiteKey(ctx, db.SetSiteKeyParams{
		OrganizationID: orgID, ExpectedVersion: int32(version), ApiKeyID: keyID, UpdatedBy: actor(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Settings{}, ErrVersionConflict
	}
	if err != nil {
		return model.Settings{}, err
	}
	return s.Get(ctx, orgID, orgSlug)
}
