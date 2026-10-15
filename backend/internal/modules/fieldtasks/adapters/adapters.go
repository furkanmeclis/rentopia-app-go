// Package adapters connects field tasks to the shared engines (bulk assign)
// and to the modules it builds on (rentals' task completion, fleet
// transfers).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/usecase"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// ActionAssign is the bulk assign action (params.user_uuid; empty =
// unassign).
const ActionAssign = "assign"

// Bulk assigns many tasks to one member (reversible: the previous assignee
// comes back).
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk { return &Bulk{svc: svc} }

// Resource implements bulkengine.BulkAdapter.
func (a *Bulk) Resource() string { return model.Resource }

// BulkActions implements bulkengine.BulkAdapter.
func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: ActionAssign, LabelKey: "bulk.actions.field_tasks.assign", Permission: rbac.PermTenantFieldTasksAssign, Reversible: true,
	}}
}

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok || id <= 0 {
		return 0, errors.New("field tasks: organization scope missing")
	}
	return id, nil
}

// ResolveTargets validates params.user_uuid (a member of the organization)
// and resolves ids or the board query.
func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	if action != ActionAssign {
		return nil, fmt.Errorf("unknown action")
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	if raw := strings.TrimSpace(target.Params["user_uuid"]); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("params.user_uuid is invalid")
		}
		if !a.svc.IsFieldMember(ctx, orgID, id) {
			return nil, fmt.Errorf("params.user_uuid: member was not found")
		}
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > usecase.MaxTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", usecase.MaxTargets)
		}
		return out, nil
	case "query":
		ids, err := a.svc.TargetUUIDs(ctx, orgID, model.FilterFromQuery(target.Query))
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.String())
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid target scope")
}

func userParam(raw string) *uuid.UUID {
	if raw = strings.TrimSpace(raw); raw == "" {
		return nil
	}
	if u, err := uuid.Parse(raw); err == nil {
		return &u
	}
	return nil
}

// ApplyItem implements bulkengine.BulkAdapter.
func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "invalid uuid"}, nil
	}
	if action != ActionAssign {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "unknown action"}, nil
	}
	current, err := a.svc.Get(ctx, orgID, id)
	if err != nil {
		return fail(entityUUID, err)
	}
	if _, err := a.svc.Assign(ctx, orgID, id, userParam(bulkengine.Params(ctx)["user_uuid"])); err != nil {
		return fail(entityUUID, err)
	}
	prev := ""
	if current.Assignee != nil {
		prev = current.Assignee.UUID.String()
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "field_task", OK: true, Op: "update", Previous: map[string]any{"user_uuid": prev},
	}, nil
}

func fail(entityUUID string, err error) (bulkengine.BulkItemResult, error) {
	var ve *usecase.ValidationError
	var se *usecase.StateError
	if errors.As(err, &ve) || errors.As(err, &se) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrForbidden) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: err.Error()}, nil
	}
	return bulkengine.BulkItemResult{}, err
}

// RevertItem gives the task back to its previous assignee.
func (a *Bulk) RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error {
	if action != ActionAssign {
		return fmt.Errorf("the action is not reversible")
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	raw, _ := previous["user_uuid"].(string)
	_, err = a.svc.Assign(ctx, orgID, id, userParam(raw))
	return err
}

// Finisher completes / cancels tasks with the rentals' rules.
type Finisher struct {
	Rentals *rentalusecase.Service
}

// FinishTask implements usecase.Finisher.
func (f Finisher) FinishTask(ctx context.Context, orgID int64, id uuid.UUID, done bool, note string) error {
	_, err := f.Rentals.FinishTask(ctx, orgID, id, done, note)
	return err
}
