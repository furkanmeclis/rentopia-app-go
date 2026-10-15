// Package adapters connects reservations to the shared engines (ioengine
// export, bulkengine cancel / assign staff) and to the modules it builds
// on without owning them: the extras equipment stock (TEC-702), campaigns
// (TEC-703) and the fleetops block hook (TEC-698). The organization always
// comes from the context (orgctx); without one every engine call fails.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	campaignsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	extrasmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// Resource is the engine slug of reservations.
const Resource = model.ResourceReservations

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = usecase.MaxListTargets

var (
	errNoOrganization = errors.New("reservations: organization scope is required")
	errNoImport       = errors.New("reservations: import is not supported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// ---------------------------------------------------------------- io engine

// IO exports reservations (list filters, branch scope of the job).
type IO struct {
	svc *usecase.Service
}

// NewIO builds the export adapter.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "reservations." + key, Type: t}
	}
	s, d := ioengine.ColumnTypeString, ioengine.ColumnTypeDatetime
	return []ioengine.Column{
		col("reference", s), col("status", s), col("payment_status", s), col("capacity_commitment", s),
		col("pickup_branch", s), col("return_branch", s), col("starts_at", d), col("ends_at", d), col("product", s),
		col("customer", s), col("channel", s), col("external_reference", s), col("total_amount", s), col("paid_amount", s),
		col("plate", s), col("plate_fixed", ioengine.ColumnTypeBoolean), col("assigned_user", s),
		col("uuid", ioengine.ColumnTypeUUID), col("created_at", d),
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := model.FilterFromQuery(query)
	out := []map[string]any{}
	for offset := int32(0); offset < int32(MaxBulkTargets); offset += 200 {
		f.Limit, f.Offset, f.SortField, f.SortDesc = 200, offset, "starts_at", false
		items, total, err := a.svc.List(ctx, orgID, f)
		if err != nil {
			return ioengine.Dataset{}, err
		}
		for _, it := range items {
			plate := ""
			if it.Vehicle != nil {
				plate = it.Vehicle.Plate
			}
			assigned := ""
			if it.AssignedUser != nil {
				assigned = strings.TrimSpace(it.AssignedUser.Name + " " + it.AssignedUser.Surname)
			}
			ext := ""
			if it.ExternalReference != nil {
				ext = *it.ExternalReference
			}
			out = append(out, map[string]any{
				"uuid": it.UUID.String(), "reference": it.Reference, "status": it.Status, "payment_status": it.PaymentStatus,
				"capacity_commitment": it.CapacityCommitment, "pickup_branch": it.PickupBranch.Code + " " + it.PickupBranch.Name,
				"return_branch": it.ReturnBranch.Code + " " + it.ReturnBranch.Name, "starts_at": it.StartsAt, "ends_at": it.EndsAt,
				"product": it.Product.Code + " " + it.Product.Title, "customer": it.Customer.DisplayName, "channel": it.Channel,
				"external_reference": ext, "total_amount": it.TotalAmount.String(), "paid_amount": it.PaidAmount.String(),
				"plate": plate, "plate_fixed": it.PlateFixed, "assigned_user": assigned, "created_at": it.CreatedAt,
			})
		}
		if int64(offset)+200 >= total {
			break
		}
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }

// -------------------------------------------------------------- bulk engine

// Bulk actions of reservations.
const (
	ActionCancel      = "cancel"
	ActionAssignStaff = "assign_staff"
)

// Bulk cancels reservations (with a reason, not reversible: capacity is
// released) and assigns a staff member (reversible).
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk { return &Bulk{svc: svc} }

func (a *Bulk) Resource() string { return Resource }

func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: ActionCancel, LabelKey: "bulk.actions.reservations.cancel", Permission: rbac.PermTenantReservationsCancel,
			Destructive: true, ConfirmKey: "bulk.confirm.reservations.cancel",
		},
		{
			ID: ActionAssignStaff, LabelKey: "bulk.actions.reservations.assign_staff", Permission: rbac.PermTenantReservationsWrite,
			Reversible: true,
		},
	}
}

// ResolveTargets validates the params (cancel: reason; assign_staff: a
// member uuid or empty to clear) and resolves the reservations.
func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	switch action {
	case ActionCancel:
		if r := strings.TrimSpace(target.Params["reason"]); len(r) < 3 || len(r) > 500 {
			return nil, fmt.Errorf("params.reason is required (3-500 characters)")
		}
	case ActionAssignStaff:
		if raw := strings.TrimSpace(target.Params["user_uuid"]); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("params.user_uuid is invalid")
			}
			if !a.svc.IsMember(ctx, orgID, id) {
				return nil, fmt.Errorf("params.user_uuid: member was not found")
			}
		}
	default:
		return nil, fmt.Errorf("unknown action")
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
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		ids, err := a.svc.TargetUUIDs(ctx, orgID, model.FilterFromQuery(target.Query), MaxBulkTargets)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	params := bulkengine.Params(ctx)
	switch action {
	case ActionCancel:
		if _, err := a.svc.Cancel(ctx, orgID, id, params["reason"]); err != nil {
			return a.fail(entityUUID, err)
		}
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: "reservation", OK: true, Op: "update"}, nil
	case ActionAssignStaff:
		current, err := a.svc.Get(ctx, orgID, id)
		if err != nil {
			return a.fail(entityUUID, err)
		}
		var user *uuid.UUID
		if raw := strings.TrimSpace(params["user_uuid"]); raw != "" {
			u, _ := uuid.Parse(raw)
			user = &u
		}
		if _, err := a.svc.Assign(ctx, orgID, id, user); err != nil {
			return a.fail(entityUUID, err)
		}
		prev := ""
		if current.AssignedUser != nil {
			prev = current.AssignedUser.UUID.String()
		}
		return bulkengine.BulkItemResult{
			EntityUUID: entityUUID, EntityType: "reservation", OK: true, Op: "update", Previous: map[string]any{"user_uuid": prev},
		}, nil
	}
	return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
}

func (a *Bulk) fail(entityUUID string, err error) (bulkengine.BulkItemResult, error) {
	var ve *usecase.ValidationError
	var se *usecase.StateError
	if errors.As(err, &ve) || errors.As(err, &se) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrConcurrent) {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
	}
	return bulkengine.BulkItemResult{}, err
}

func (a *Bulk) RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error {
	if action != ActionAssignStaff {
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
	var user *uuid.UUID
	if raw, _ := previous["user_uuid"].(string); raw != "" {
		if u, err := uuid.Parse(raw); err == nil {
			user = &u
		}
	}
	_, err = a.svc.Assign(ctx, orgID, id, user)
	return err
}

// ----------------------------------------------------------- module ports

// Affected adapts reservations to fleetops' block hook.
type Affected struct {
	svc *usecase.Service
}

// NewAffected builds the block hook adapter.
func NewAffected(svc *usecase.Service) *Affected { return &Affected{svc: svc} }

// AllocationsAffected implements fleetopsusecase.AffectedHandler.
func (a *Affected) AllocationsAffected(ctx context.Context, orgID int64, sourceType string, source uuid.UUID, allocations []uuid.UUID) {
	a.svc.AllocationsAffected(ctx, orgID, sourceType, source, allocations)
}

// Equipment holds physical extras through the extras module (TEC-702),
// tied to the reservation's vehicle allocation.
type Equipment struct {
	svc *extrasusecase.Service
}

// NewEquipment builds the equipment adapter.
func NewEquipment(svc *extrasusecase.Service) *Equipment { return &Equipment{svc: svc} }

// ReserveEquipment implements model.EquipmentReserver.
func (a *Equipment) ReserveEquipment(ctx context.Context, orgID int64, in model.EquipmentRequest) (uuid.UUID, error) {
	extra, qty, va, ref := in.ExtraUUID, in.Quantity, in.VehicleAllocationUUID, in.ReservationUUID
	alloc, err := a.svc.Reserve(ctx, orgID, extrasmodel.ReserveInput{
		ExtraUUID: &extra, Quantity: &qty, VehicleAllocationUUID: &va, RefType: model.RefType, RefUUID: &ref,
		Note: "reservation",
	}, in.ActorID)
	if err != nil {
		var ve *extrasusecase.ValidationError
		if errors.As(err, &ve) {
			for _, d := range ve.Details {
				if d.Code == "not_physical" {
					return uuid.Nil, nil
				}
			}
		}
		var se *extrasusecase.StockError
		if errors.As(err, &se) {
			return uuid.Nil, &model.EquipmentError{ExtraUUID: extra, Stock: se.Stock, Available: se.Available, Requested: se.Requested}
		}
		return uuid.Nil, err
	}
	return alloc.UUID, nil
}

// ReleaseEquipment implements model.EquipmentReserver.
func (a *Equipment) ReleaseEquipment(ctx context.Context, orgID int64, id uuid.UUID, reason string) error {
	_, err := a.svc.Release(ctx, orgID, id, reason, nil)
	if errors.Is(err, extrasusecase.ErrNotActive) || errors.Is(err, extrasusecase.ErrNotFound) {
		return nil
	}
	return err
}

// Coupons redeems campaigns through the campaigns module (TEC-703).
type Coupons struct {
	svc *campaignsusecase.Service
}

// NewCoupons builds the campaigns adapter.
func NewCoupons(svc *campaignsusecase.Service) *Coupons { return &Coupons{svc: svc} }

// Redeem implements model.CouponRedeemer.
func (a *Coupons) Redeem(ctx context.Context, orgID int64, in model.CouponRedemption) error {
	_, err := a.svc.Redeem(ctx, orgID, campaignsusecase.RedeemInput{
		Quote: campaignsusecase.Quote{
			Code: in.Code, CampaignUUID: in.CampaignUUID, ProductID: in.ProductID, SegmentID: in.SegmentID, Channel: in.Channel,
			Subtotal: in.Subtotal, CustomerUUID: in.CustomerUUID,
		},
		ReservationUUID: in.ReservationUUID, ActorID: in.ActorID,
	})
	var re *campaignsusecase.RejectionError
	if errors.As(err, &re) {
		return &model.CouponError{Reason: re.Reason, Params: re.Params}
	}
	if errors.Is(err, campaignsusecase.ErrAlreadyRedeemed) {
		return &model.CouponError{Reason: "already_redeemed"}
	}
	return err
}

// Release implements model.CouponRedeemer.
func (a *Coupons) Release(ctx context.Context, orgID int64, reservation uuid.UUID, reason string) error {
	_, err := a.svc.Release(ctx, orgID, reservation, reason)
	if errors.Is(err, campaignsusecase.ErrRedemptionNotFound) {
		return nil
	}
	return err
}

// Consume implements model.CouponRedeemer.
func (a *Coupons) Consume(ctx context.Context, orgID int64, reservation uuid.UUID) error {
	_, err := a.svc.Consume(ctx, orgID, reservation)
	if errors.Is(err, campaignsusecase.ErrRedemptionNotFound) {
		return nil
	}
	return err
}
