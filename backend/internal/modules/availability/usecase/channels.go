package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
)

// Channel capacity rules (docs/design/availability-engine.md §6). Physical
// capacity comes from the engine; the rules only decide how much of it a
// channel may sell:
//
//	avail    = physical - reserved for other channels (their unused quotas)
//	own      = min(own unused quota, avail)
//	shared   = max(0, avail - own unused quota - safety margin)
//	sellable = min(own + shared, sales limit remaining)
//
// Unused quotas are scaled down proportionally when together they exceed
// the physical capacity, so a quota never promises a vehicle that does not
// exist and over-subscribed quotas do not block each other.

type ruleRow = db.ListActiveCapacityRulesRow

// specificity orders matching rules: product, then branch, then channel.
func specificity(r ruleRow) int {
	n := 0
	if r.RentalProductID.Valid {
		n += 4
	}
	if r.BranchID.Valid {
		n += 2
	}
	if r.Channel.Valid {
		n++
	}
	return n
}

func applies(r ruleRow, productID, branchID int64) bool {
	return (!r.RentalProductID.Valid || r.RentalProductID.Int64 == productID) &&
		(!r.BranchID.Valid || r.BranchID.Int64 == branchID)
}

// channelRules are the most specific rules of each kind for one product and
// pick-up branch.
type channelRules struct {
	margin map[string]ruleRow // channel ("" = every channel) → margin rule
	quota  map[string]ruleRow
	limit  map[string]ruleRow
}

func pickRules(rules []ruleRow, productID, branchID int64) channelRules {
	out := channelRules{margin: map[string]ruleRow{}, quota: map[string]ruleRow{}, limit: map[string]ruleRow{}}
	for _, r := range rules {
		if !applies(r, productID, branchID) {
			continue
		}
		var m map[string]ruleRow
		switch r.RuleType {
		case model.RuleSafetyMargin:
			m = out.margin
		case model.RuleGuaranteedQuota:
			m = out.quota
		case model.RuleSalesLimit:
			m = out.limit
		default:
			continue
		}
		key := r.Channel.String
		if cur, ok := m[key]; !ok || specificity(r) > specificity(cur) {
			m[key] = r
		}
	}
	return out
}

func (c channelRules) empty() bool {
	return len(c.margin) == 0 && len(c.quota) == 0 && len(c.limit) == 0
}

// marginFor is the margin of a channel: its own rule, else the rule for
// every channel, the more specific of both.
func (c channelRules) marginFor(channel string) int {
	own, okOwn := c.margin[channel]
	all, okAll := c.margin[""]
	switch {
	case okOwn && okAll:
		if specificity(all) > specificity(own) {
			return int(all.Quantity)
		}
		return int(own.Quantity)
	case okOwn:
		return int(own.Quantity)
	case okAll:
		return int(all.Quantity)
	}
	return 0
}

// usage is the peak number of concurrent commitments of a product per
// channel (optionally per pick-up branch) inside a range.
type usage struct {
	rows []db.ListProductCommitmentsRow
}

func (u usage) peak(channel string, branchID *int64) int {
	type ev struct {
		at    time.Time
		delta int
	}
	evs := []ev{}
	for _, r := range u.rows {
		if r.Channel != channel || (branchID != nil && r.PickupBranchID != *branchID) {
			continue
		}
		evs = append(evs, ev{r.StartsAt.Time, 1}, ev{r.EndsAt.Time, -1})
	}
	sort.Slice(evs, func(i, j int) bool {
		if !evs[i].at.Equal(evs[j].at) {
			return evs[i].at.Before(evs[j].at)
		}
		return evs[i].delta < evs[j].delta // ends before starts (half-open)
	})
	cur, best := 0, 0
	for _, e := range evs {
		cur += e.delta
		if cur > best {
			best = cur
		}
	}
	return best
}

func ruleBranch(r ruleRow) *int64 {
	if r.BranchID.Valid {
		id := r.BranchID.Int64
		return &id
	}
	return nil
}

// channelEval is the channel side of a breakdown.
type channelEval struct {
	margin         int
	unusedOthers   int
	ownUnused      *int
	limitRemaining *int
}

func evaluate(rules channelRules, u usage, channel string) channelEval {
	e := channelEval{margin: rules.marginFor(channel)}
	for ch, r := range rules.quota {
		unused := int(r.Quantity) - u.peak(ch, ruleBranch(r))
		if unused < 0 {
			unused = 0
		}
		if ch == channel {
			e.ownUnused = &unused
		} else {
			e.unusedOthers += unused
		}
	}
	if r, ok := rules.limit[channel]; ok {
		rem := int(r.Quantity) - u.peak(channel, ruleBranch(r))
		if rem < 0 {
			rem = 0
		}
		e.limitRemaining = &rem
	}
	return e
}

// apply turns physical capacity into the channel's breakdown fields.
func (e channelEval) apply(physical int, b *model.Breakdown) {
	own := 0
	if e.ownUnused != nil {
		own = *e.ownUnused
	}
	others := e.unusedOthers
	if total := own + others; total > physical && total > 0 {
		// Over-subscribed quotas share the real capacity proportionally.
		own = own * physical / total
		others = others * physical / total
	}
	avail := max(0, physical-others)
	ownPart := min(own, avail)
	shared := max(0, avail-own-e.margin)
	sellable := ownPart + shared
	if e.limitRemaining != nil {
		sellable = min(sellable, *e.limitRemaining)
	}
	b.SafetyMargin = e.margin
	b.ReservedForOtherChannels = others
	if e.ownUnused != nil {
		v := own
		b.OwnQuotaRemaining = &v
	}
	b.ChannelLimitRemaining = e.limitRemaining
	b.Sellable = sellable
}

// needed is the physical capacity the channel needs to sell one more unit.
func (e channelEval) needed() int {
	if e.limitRemaining != nil && *e.limitRemaining <= 0 {
		return -1 // the limit is reached whatever the capacity
	}
	if e.ownUnused != nil && *e.ownUnused > 0 {
		return e.unusedOthers + 1
	}
	return e.unusedOthers + e.margin + 1
}

func (s *Service) productUsage(ctx context.Context, q *db.Queries, orgID, productID int64, r resolved, now time.Time) (usage, error) {
	rows, err := q.ListProductCommitments(ctx, db.ListProductCommitmentsParams{
		OrganizationID: orgID, RentalProductID: productID, NowAt: ts(now), FromAt: ts(r.start), ToAt: ts(r.end),
	})
	return usage{rows: rows}, err
}
