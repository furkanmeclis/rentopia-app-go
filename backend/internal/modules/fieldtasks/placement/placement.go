// Package placement derives where a handover / return task happens from
// its reservation (TEC-724): a delivery address, a service point or the
// branch, with the contact and meeting details of the delivery. Rentals
// call Locate when they open a handover / return task, so a task opened by
// the rental flow (a re-created handover, a swap's new return task) carries
// the same place as the delivery task the reservation opened.
package placement

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Place is the place, contact and meeting details of a task.
type Place struct {
	Kind         string
	PointID      pgtype.Int8
	Address      string
	District     string
	City         string
	Latitude     pgtype.Float8
	Longitude    pgtype.Float8
	ContactName  string
	ContactPhone string
	FlightNumber string
	MeetingNote  string
	// Delivery: staff go to the customer (a delivery point or an address).
	Delivery bool
}

// ParseDelivery reads a stored delivery JSON (nil when empty or invalid).
func ParseDelivery(raw []byte) *model.Delivery {
	if len(raw) == 0 {
		return nil
	}
	var d model.Delivery
	if json.Unmarshal(raw, &d) != nil {
		return nil
	}
	d.Normalize()
	if d.Empty() {
		return nil
	}
	return &d
}

// Of returns the place of one side (pick-up or return) of a reservation.
func Of(point pgtype.Int8, pointDelivery bool, deliveryJSON []byte) Place {
	d := ParseDelivery(deliveryJSON)
	p := Place{Kind: model.LocationBranch}
	if point.Valid {
		p.Kind, p.PointID = model.LocationPoint, point
	}
	if d != nil {
		if d.Address != "" {
			p.Kind = model.LocationAddress
			p.Address, p.District, p.City = d.Address, d.District, d.City
			if d.Latitude != nil && d.Longitude != nil {
				p.Latitude = pgtype.Float8{Float64: *d.Latitude, Valid: true}
				p.Longitude = pgtype.Float8{Float64: *d.Longitude, Valid: true}
			}
		}
		p.ContactName, p.ContactPhone = d.ContactName, d.ContactPhone
		p.FlightNumber, p.MeetingNote = d.FlightNumber, d.MeetingNote
	}
	p.Delivery = (point.Valid && pointDelivery) || p.Kind == model.LocationAddress
	return p
}

// Params is the SetOperationTaskPlace update of a task.
func (p Place) Params(orgID, taskID int64, actor pgtype.Int8) db.SetOperationTaskPlaceParams {
	return db.SetOperationTaskPlaceParams{
		OrganizationID: orgID, ID: taskID, LocationKind: p.Kind, PointID: p.PointID, Address: p.Address,
		District: p.District, City: p.City, Latitude: p.Latitude, Longitude: p.Longitude,
		ContactName: p.ContactName, ContactPhone: p.ContactPhone, FlightNumber: p.FlightNumber,
		MeetingNote: p.MeetingNote, ActorID: actor,
	}
}

// Locate sets the place of a live handover / return task from its
// reservation (no-op for other types, tasks without a reservation and
// unchanged places).
func Locate(ctx context.Context, q *db.Queries, orgID int64, task uuid.UUID, actor pgtype.Int8) error {
	f, err := q.GetTaskPlacementFacts(ctx, db.GetTaskPlacementFactsParams{OrganizationID: orgID, Uuid: task})
	if err != nil {
		// No reservation (or no task): nothing to place.
		return nil
	}
	var p Place
	switch f.TaskType {
	case model.TypeHandover:
		p = Of(f.PickupPointID, f.PickupPointDelivery, f.PickupDelivery)
	case model.TypeReturn:
		p = Of(f.ReturnPointID, f.ReturnPointDelivery, f.ReturnDelivery)
	default:
		return nil
	}
	return q.SetOperationTaskPlace(ctx, p.Params(orgID, f.ID, actor))
}
