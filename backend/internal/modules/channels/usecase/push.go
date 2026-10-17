package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// istanbul is the zone of pick-up days.
var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("TRT", 3*3600)
	}
	return loc
}()

// pickupHour is the reference pick-up time of a day's one-day rental.
const pickupHour = 10

// candidate is one internal product offered under an external code at a
// location.
type candidate struct {
	mappingID   int64
	productID   int64
	productUUID uuid.UUID
	published   bool
	stopSale    bool
	maxQuota    *int32
	priority    int32
}

// slot is one external product code at one location.
type slot struct {
	code       string
	location   string
	branchID   int64
	branchUUID uuid.UUID
	candidates []candidate
}

// buildSlots groups the account's product mappings by external code and
// location. A product mapping without a branch is offered at every mapped
// location; several products may share a code (lowest priority first).
func buildSlots(rows []db.ListActiveChannelMappingsRow) []*slot {
	type loc struct {
		id   int64
		uuid uuid.UUID
		code string
	}
	locations := map[int64]loc{}
	order := []int64{}
	for _, r := range rows {
		if r.EntityType == model.EntityBranch && r.BranchID.Valid && r.BranchActive.Valid && r.BranchActive.Bool {
			if _, ok := locations[r.BranchID.Int64]; !ok {
				order = append(order, r.BranchID.Int64)
			}
			locations[r.BranchID.Int64] = loc{id: r.BranchID.Int64, uuid: r.BranchUuid.Bytes, code: r.ExternalCode}
		}
	}
	slots := map[string]*slot{}
	keys := []string{}
	for _, r := range rows {
		if r.EntityType != model.EntityProduct || !r.RentalProductID.Valid || !r.ProductUuid.Valid {
			continue
		}
		targets := order
		if r.BranchID.Valid {
			targets = []int64{r.BranchID.Int64}
		}
		for _, bid := range targets {
			l, ok := locations[bid]
			if !ok {
				continue
			}
			key := r.ExternalCode + "\x00" + l.code
			sl, ok := slots[key]
			if !ok {
				sl = &slot{code: r.ExternalCode, location: l.code, branchID: l.id, branchUUID: l.uuid}
				slots[key] = sl
				keys = append(keys, key)
			}
			sl.candidates = append(sl.candidates, candidate{
				mappingID: r.ID, productID: r.RentalProductID.Int64, productUUID: r.ProductUuid.Bytes,
				published: textOr(r.ProductStatus) == "published", stopSale: r.StopSale, maxQuota: int4Ptr(r.MaxQuota),
				priority: r.Priority,
			})
		}
	}
	sort.Strings(keys)
	out := make([]*slot, 0, len(keys))
	for _, k := range keys {
		sl := slots[k]
		sort.SliceStable(sl.candidates, func(i, j int) bool { return sl.candidates[i].priority < sl.candidates[j].priority })
		out = append(out, sl)
	}
	return out
}

// dayWindow is the one-day rental priced for a pick-up day: 10:00 local
// (later today when that has passed), 24 hours. ok = false when the day
// can no longer be sold.
func dayWindow(day time.Time, now time.Time) (time.Time, time.Time, bool) {
	start := time.Date(day.Year(), day.Month(), day.Day(), pickupHour, 0, 0, 0, istanbul)
	earliest := now.Add(time.Hour).Truncate(time.Hour).Add(time.Hour)
	if start.Before(earliest) {
		start = earliest.In(istanbul)
	}
	if start.Year() != day.Year() || start.YearDay() != day.YearDay() {
		return time.Time{}, time.Time{}, false
	}
	return start.UTC(), start.Add(24 * time.Hour).UTC(), true
}

// offerRef is a deterministic reference of what an item stands for.
func offerRef(accountID, productID int64, version int32, branchID int64, date string, price string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%d|%d|%s|%s", accountID, productID, version, branchID, date, price)))
	return "OF" + hex.EncodeToString(h[:])[:22]
}

// push builds and sends the inventory snapshot of an account.
func (s *Service) push(ctx context.Context, orgID, accountID int64, trigger string) (uuid.UUID, error) {
	sess, err := s.begin(ctx, orgID, accountID, model.DirectionPush, trigger)
	if err != nil {
		return uuid.Nil, err
	}
	res := runResult{status: model.RunFailed, details: map[string]any{}}
	defer func() { s.finish(ctx, sess, res) }()
	caps := sess.provider.Capabilities()
	if !caps.Availability {
		res.err = ErrUnsupported.Error()
		s.savePush(ctx, sess, model.RunFailed, res.err, false, nil)
		return sess.runUUID, ErrUnsupported
	}
	rows, err := s.q.ListActiveChannelMappings(ctx, db.ListActiveChannelMappingsParams{OrganizationID: orgID, IntegrationAccountID: accountID})
	if err != nil {
		res.err = err.Error()
		return sess.runUUID, err
	}
	slots := buildSlots(rows)
	items, failedDays, availErr, err := s.inventory(ctx, sess, slots)
	if err != nil {
		res.err = err.Error()
		s.savePush(ctx, sess, model.RunFailed, res.err, false, nil)
		return sess.runUUID, err
	}
	validUntil := s.clock().Add(time.Duration(sess.row.ValidityMinutes) * time.Minute)
	push := channels.InventoryPush{
		Items: items, ValidUntil: validUntil,
		Offline: channels.OfflinePolicy{Mode: sess.row.OfflinePolicy, Quota: int(sess.row.OfflineQuota)},
	}
	if push.Offline.Mode == channels.OfflineClose {
		push.Offline.Quota = 0
	}
	res.total = len(items)
	res.details["slots"] = len(slots)
	res.details["horizon_days"] = sess.row.HorizonDays
	res.details["valid_until"] = validUntil
	res.details["offline_policy"] = push.Offline.Mode
	if failedDays > 0 {
		res.details["unpriced_items"] = failedDays
	}
	if availErr != "" {
		// Typically the sales channel is not active in the settings.
		res.details["availability_error"] = truncate(availErr, 500)
	}
	receipt, err := s.sendPush(ctx, sess, push, caps.MaxItemsPerPush)
	res.details["references"] = receipt.Reference
	if len(receipt.Errors) > 0 {
		n := min(len(receipt.Errors), 20)
		res.details["item_errors"] = receipt.Errors[:n]
	}
	if err != nil {
		res.err = err.Error()
		s.savePush(ctx, sess, model.RunFailed, res.err, false, nil)
		return sess.runUUID, err
	}
	res.ok, res.failed = receipt.Accepted, receipt.Rejected
	res.status = model.RunSucceeded
	if receipt.Rejected > 0 {
		res.status = model.RunPartial
	}
	s.savePush(ctx, sess, res.status, "", true, &validUntil)
	_, _ = s.q.DeleteExpiredChannelOffers(ctx, db.DeleteExpiredChannelOffersParams{
		OrganizationID: orgID, Before: pgtype.Date{Time: s.clock().AddDate(0, 0, -60), Valid: true},
	})
	return sess.runUUID, nil
}

// sendPush sends the snapshot in pages of the adapter's size.
func (s *Service) sendPush(ctx context.Context, sess *session, push channels.InventoryPush, page int) (channels.PushReceipt, error) {
	if page <= 0 || len(push.Items) <= page {
		r, err := sess.provider.PushInventory(ctx, sess.account, push)
		return r, err
	}
	out := channels.PushReceipt{Errors: []channels.ItemError{}}
	refs := ""
	for from := 0; from < len(push.Items); from += page {
		to := min(from+page, len(push.Items))
		part := push
		part.Items = push.Items[from:to]
		r, err := sess.provider.PushInventory(ctx, sess.account, part)
		if err != nil {
			return out, err
		}
		out.Accepted += r.Accepted
		out.Rejected += r.Rejected
		for _, e := range r.Errors {
			e.Index += from
			out.Errors = append(out.Errors, e)
		}
		if refs != "" {
			refs += ","
		}
		refs += r.Reference
	}
	out.Reference = refs
	return out, nil
}

func (s *Service) savePush(ctx context.Context, sess *session, status, errText string, ok bool, validUntil *time.Time) {
	if err := s.q.SaveChannelPushState(context.WithoutCancel(ctx), db.SaveChannelPushStateParams{
		OrganizationID: sess.orgID, IntegrationAccountID: sess.row.IntegrationAccountID, At: ts(s.clock()), Status: status,
		Error: truncate(errText, 2000), Ok: ok, ValidUntil: tsPtr(validUntil), StartedAt: ts(sess.started),
	}); err != nil {
		s.log.Warn("channel_push_state_failed", "organization_id", sess.orgID, "error", err)
	}
}

// inventory computes the items of every slot and day of the horizon and
// records the offer behind each item. Returns the items and how many could
// not be priced / checked (sent closed) with the last availability error.
func (s *Service) inventory(ctx context.Context, sess *session, slots []*slot) ([]channels.InventoryItem, int, string, error) {
	items := []channels.InventoryItem{}
	if len(slots) == 0 {
		return items, 0, "", nil
	}
	availErr := ""
	now := s.clock()
	today := now.In(istanbul)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, istanbul)
	// Products per branch: one availability call per branch and day.
	byBranch := map[int64][]uuid.UUID{}
	branchUUID := map[int64]uuid.UUID{}
	for _, sl := range slots {
		branchUUID[sl.branchID] = sl.branchUUID
		for _, c := range sl.candidates {
			if !containsUUID(byBranch[sl.branchID], c.productUUID) {
				byBranch[sl.branchID] = append(byBranch[sl.branchID], c.productUUID)
			}
		}
	}
	failed := 0
	accountStop := sess.row.StopSale
	for d := 0; d < int(sess.row.HorizonDays); d++ {
		day := today.AddDate(0, 0, d)
		date := day.Format("2006-01-02")
		start, end, ok := dayWindow(day, now)
		avail := map[int64]map[uuid.UUID]Availability{}
		if ok {
			for bid, products := range byBranch {
				a, err := s.booker.Availability(ctx, sess.orgID, sess.row.Channel, branchUUID[bid], start, end, products)
				if err != nil {
					if ctx.Err() != nil {
						return nil, failed, availErr, ctx.Err()
					}
					availErr = err.Error()
					continue
				}
				avail[bid] = a
			}
		}
		for _, sl := range slots {
			item := channels.InventoryItem{ProductCode: sl.code, LocationCode: sl.location, Date: date, Currency: "TRY"}
			branchAvail, known := avail[sl.branchID]
			var chosen *candidate
			var chosenAvail Availability
			for i := range sl.candidates {
				c := &sl.candidates[i]
				a, has := branchAvail[c.productUUID]
				if !has || !c.published {
					continue
				}
				if chosen == nil {
					chosen, chosenAvail = c, a
				}
				if !c.stopSale && a.Sellable > 0 {
					chosen, chosenAvail = c, a
					break
				}
			}
			if !ok || !known || chosen == nil {
				if ok {
					failed++
				}
				item.StopSale = true
				items = append(items, item)
				continue
			}
			units := chosenAvail.Sellable
			if chosen.maxQuota != nil && int(*chosen.maxQuota) < units {
				units = int(*chosen.maxQuota)
			}
			if accountStop || chosen.stopSale {
				units, item.StopSale = 0, true
			}
			item.Available = max(units, 0)
			price := ""
			if chosenAvail.HasPrice {
				item.HasPrice, item.DailyPrice = true, chosenAvail.DailyPrice
				if chosenAvail.Currency != "" {
					item.Currency = chosenAvail.Currency
				}
				price = chosenAvail.DailyPrice.String()
			} else if !item.StopSale {
				// Never sell without a price.
				item.Available, item.StopSale = 0, true
				failed++
			}
			item.OfferRef = offerRef(sess.row.IntegrationAccountID, chosen.productID, chosenAvail.Version, sl.branchID, date, price)
			op := db.UpsertChannelOfferParams{
				OrganizationID: sess.orgID, IntegrationAccountID: sess.row.IntegrationAccountID, OfferRef: item.OfferRef,
				MappingID: chosen.mappingID, RentalProductID: chosen.productID, ProductVersion: chosenAvail.Version,
				BranchID: sl.branchID, ExternalCode: sl.code, LocationCode: sl.location,
				StayDate: pgtype.Date{Time: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Valid: true},
				Currency: item.Currency, Available: int32(item.Available),
			}
			if item.HasPrice {
				op.DailyPrice = item.DailyPrice.Numeric()
			}
			if err := s.q.UpsertChannelOffer(ctx, op); err != nil {
				return nil, failed, availErr, err
			}
			items = append(items, item)
		}
	}
	return items, failed, availErr, nil
}

func containsUUID(list []uuid.UUID, v uuid.UUID) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
