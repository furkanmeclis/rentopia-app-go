package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func jsonMap(raw []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func messageView(r db.GetChannelMessageRow) model.Message {
	m := model.Message{
		UUID: r.Uuid, Account: model.Ref{UUID: r.AccountUuid, Name: r.AccountName, Provider: r.AccountProvider},
		Channel: r.Channel, MessageKey: r.MessageKey, ExternalRef: r.ExternalRef, Type: r.MessageType, Version: r.Version,
		OccurredAt: timePtr(r.OccurredAt), Source: r.Source, Status: r.Status, ErrorCode: r.ErrorCode,
		ErrorMessage: r.ErrorMessage, Attempts: r.Attempts, Deliveries: r.Deliveries, ProcessedAt: timePtr(r.ProcessedAt),
		AckedAt: timePtr(r.AckedAt), AckError: r.AckError, Summary: jsonMap(r.Summary), ResolutionNote: r.ResolutionNote,
		ResolvedAt: timePtr(r.ResolvedAt), CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.ReservationUuid.Valid {
		m.Reservation = &model.Ref{UUID: r.ReservationUuid.Bytes, Reference: textOr(r.ReservationReference), Status: textOr(r.ReservationStatus)}
	}
	return m
}

// ListMessages lists a page of inbound messages.
func (s *Service) ListMessages(ctx context.Context, orgID int64, f model.MessageFilter) ([]model.Message, int64, error) {
	p := db.ListChannelMessagesParams{
		OrganizationID: orgID, Statuses: f.Statuses, Types: f.Types, SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.AccountUUID != nil {
		p.AccountUuid = pgtype.UUID{Bytes: *f.AccountUUID, Valid: true}
	}
	if v := strings.TrimSpace(f.ExternalRef); v != "" {
		p.ExternalRef = pgtype.Text{String: v, Valid: true}
	}
	if v := strings.TrimSpace(f.Q); v != "" {
		p.Q = pgtype.Text{String: v, Valid: true}
	}
	rows, err := s.q.ListChannelMessages(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Message, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, messageView(db.GetChannelMessageRow{
			Uuid: r.Uuid, Channel: r.Channel, MessageKey: r.MessageKey, ExternalRef: r.ExternalRef, MessageType: r.MessageType,
			Version: r.Version, OccurredAt: r.OccurredAt, Source: r.Source, Status: r.Status, ErrorCode: r.ErrorCode,
			ErrorMessage: r.ErrorMessage, Attempts: r.Attempts, Deliveries: r.Deliveries, ProcessedAt: r.ProcessedAt,
			AckedAt: r.AckedAt, AckError: r.AckError, Summary: r.Summary, ResolutionNote: r.ResolutionNote,
			ResolvedAt: r.ResolvedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, AccountUuid: r.AccountUuid,
			AccountName: r.AccountName, AccountProvider: r.AccountProvider, BookingUuid: r.BookingUuid,
			ReservationUuid: r.ReservationUuid, ReservationReference: r.ReservationReference,
		}))
	}
	return out, total, nil
}

// GetMessage returns a message with its booking and the actions allowed.
func (s *Service) GetMessage(ctx context.Context, orgID int64, id uuid.UUID) (model.MessageDetail, error) {
	r, err := s.q.GetChannelMessage(ctx, db.GetChannelMessageParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.MessageDetail{}, ErrMessageNotFound
	}
	if err != nil {
		return model.MessageDetail{}, err
	}
	d := model.MessageDetail{Message: messageView(r)}
	if r.BookingUuid.Valid {
		b := &model.Booking{
			UUID: r.BookingUuid.Bytes, Status: textOr(r.BookingStatus), AppliedVersion: r.BookingAppliedVersion.Int64,
			LastVersion: r.BookingLastVersion.Int64, CollectedBy: textOr(r.BookingCollectedBy), Currency: textOr(r.BookingCurrency),
			Gross: money.FromNumericOrZero(r.BookingGross), Commission: money.FromNumericOrZero(r.BookingCommission),
			Net: money.FromNumericOrZero(r.BookingNet),
		}
		if r.BookingPriceDifference.Valid {
			v := money.FromNumericOrZero(r.BookingPriceDifference)
			b.PriceDifference = &v
		}
		d.Booking = b
	}
	if r.ResolvedByName.Valid {
		d.ResolvedBy = strings.TrimSpace(r.ResolvedByName.String + " " + r.ResolvedBySurname.String)
	}
	sc, _ := orgctx.From(ctx)
	can := sc.Has(rbac.PermTenantChannelsSync) && sc.AllBranches()
	d.Actions = model.MessageActions{Retry: can && r.Status == model.MessageFailed, Ignore: can && r.Status == model.MessageFailed}
	return d, nil
}

// MessageFacets counts messages per status and type.
func (s *Service) MessageFacets(ctx context.Context, orgID int64) (model.MessageFacets, error) {
	rows, err := s.q.ChannelMessageFacets(ctx, orgID)
	if err != nil {
		return model.MessageFacets{}, err
	}
	out := model.MessageFacets{Statuses: map[string]int64{}, Types: map[string]int64{}}
	for _, v := range model.MessageStatuses {
		out.Statuses[v] = 0
	}
	for _, v := range model.MessageTypes {
		out.Types[v] = 0
	}
	for _, r := range rows {
		out.Statuses[r.Status] += r.Total
		out.Types[r.MessageType] += r.Total
	}
	out.NeedsAction = out.Statuses[model.MessageFailed]
	return out, nil
}

// ListSyncRuns lists a page of the sync log.
func (s *Service) ListSyncRuns(ctx context.Context, orgID int64, f model.SyncRunFilter) ([]model.SyncRun, int64, error) {
	return s.listRuns(ctx, orgID, f, nil)
}

func (s *Service) listRuns(ctx context.Context, orgID int64, f model.SyncRunFilter, only *uuid.UUID) ([]model.SyncRun, int64, error) {
	p := db.ListChannelSyncRunsParams{
		OrganizationID: orgID, Directions: f.Directions, Statuses: f.Statuses, SortDesc: f.SortDesc || only != nil,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.AccountUUID != nil {
		p.AccountUuid = pgtype.UUID{Bytes: *f.AccountUUID, Valid: true}
	}
	if only != nil {
		p.RunUuid = pgtype.UUID{Bytes: *only, Valid: true}
	}
	rows, err := s.q.ListChannelSyncRuns(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := []model.SyncRun{}
	var total int64
	for _, r := range rows {
		total = r.Total
		run := model.SyncRun{
			UUID: r.Uuid, Account: model.Ref{UUID: r.AccountUuid, Name: r.AccountName, Provider: r.AccountProvider},
			Direction: r.Direction, Trigger: r.Trigger, Status: r.Status, ItemsTotal: r.ItemsTotal, ItemsOK: r.ItemsOk,
			ItemsFailed: r.ItemsFailed, Error: r.Error, Details: jsonMap(r.Details), StartedAt: r.StartedAt.Time.UTC(),
			FinishedAt: timePtr(r.FinishedAt),
		}
		if r.ActorName.Valid {
			run.Actor = strings.TrimSpace(r.ActorName.String + " " + r.ActorSurname.String)
		}
		out = append(out, run)
	}
	return out, total, nil
}

// Reconcile builds the channel vs Rentopia report of bookings picked up in
// [from, to): the channel's statement (when the adapter has one) against
// the channel bookings and their reservations. Each booking is matched,
// missing on one side, or differs in status, amount or commission. The
// run is written to the sync log.
func (s *Service) Reconcile(ctx context.Context, orgID int64, id uuid.UUID, from, to time.Time) (model.Reconciliation, error) {
	r, err := s.accountRow(ctx, orgID, id)
	if err != nil {
		return model.Reconciliation{}, err
	}
	if !to.After(from) || to.Sub(from) > 93*24*time.Hour {
		return model.Reconciliation{}, validation("to", "invalid", "the period must be positive and at most 93 days")
	}
	out := model.Reconciliation{
		Account: model.Ref{UUID: r.Uuid, Name: r.DisplayName, Provider: r.Provider}, From: from.Format("2006-01-02"),
		To: to.Format("2006-01-02"), Source: "local", Counts: map[string]int{}, Lines: []model.ReconciliationLine{}, Currency: "TRY",
		Totals: model.ReconciliationTotals{
			ChannelGross: money.Zero(), ChannelCommission: money.Zero(), ChannelNet: money.Zero(), LocalGross: money.Zero(),
			LocalCommission: money.Zero(), CollectedByChannel: money.Zero(), CollectedByUs: money.Zero(),
		},
	}
	for _, m := range model.MatchStatuses {
		out.Counts[m] = 0
	}
	local, err := s.q.ListChannelReservationsForReconciliation(ctx, db.ListChannelReservationsForReconciliationParams{
		OrganizationID: orgID, IntegrationAccountID: r.ID, FromAt: ts(from), ToAt: ts(to),
	})
	if err != nil {
		return out, err
	}
	var statement []channels.StatementLine
	var sess *session
	if p, ok := s.providers[r.Provider]; ok && r.Status == "active" && p.Capabilities().Statement {
		sess, err = s.begin(ctx, orgID, r.ID, model.DirectionReconcile, model.TriggerManual)
		if err == nil {
			statement, err = p.Statement(ctx, sess.account, from, to)
			if err != nil {
				out.Warning = "statement_unavailable: " + err.Error()
				statement = nil
			} else {
				out.Source = "statement"
			}
		} else {
			out.Warning = "statement_unavailable: " + err.Error()
			sess = nil
		}
	}
	byRef := map[string]db.ListChannelReservationsForReconciliationRow{}
	for _, l := range local {
		byRef[l.ExternalRef] = l
	}
	seen := map[string]bool{}
	add := func(line model.ReconciliationLine) {
		out.Counts[line.Match]++
		out.Lines = append(out.Lines, line)
	}
	if out.Source == "statement" {
		for _, st := range statement {
			seen[st.ExternalRef] = true
			line := model.ReconciliationLine{ExternalRef: st.ExternalRef, ChannelStatus: st.Status, CollectedBy: st.CollectedBy}
			start := st.StartsAt.UTC()
			line.StartsAt = &start
			total, commission, net := st.Total, st.Commission, st.Net
			line.ChannelTotal, line.ChannelCommission, line.ChannelNet = &total, &commission, &net
			if st.Status != channels.StatusCancelled {
				s.sumChannel(&out.Totals, st.CollectedBy, total, commission, net)
			}
			l, ok := byRef[st.ExternalRef]
			if !ok {
				line.Match = model.MatchMissingLocal
				if st.Status == channels.StatusCancelled {
					line.Match = model.MatchOK
				}
				add(line)
				continue
			}
			s.compare(&out.Totals, &line, l, st.Status, total, commission)
			add(line)
		}
	}
	for _, l := range local {
		if seen[l.ExternalRef] {
			continue
		}
		line := model.ReconciliationLine{ExternalRef: l.ExternalRef, ChannelStatus: l.ChannelStatus, CollectedBy: l.CollectedBy, ErrorCode: l.LastErrorCode}
		line.StartsAt = timePtr(l.StartsAt)
		gross, commission, net := money.FromNumericOrZero(l.GrossAmount), money.FromNumericOrZero(l.CommissionAmount), money.FromNumericOrZero(l.NetAmount)
		line.ChannelTotal, line.ChannelCommission, line.ChannelNet = &gross, &commission, &net
		if out.Source == "statement" {
			// The channel does not list a booking we hold.
			line.Match = model.MatchMissingRemote
			s.compareLocalOnly(&out.Totals, &line, l)
			add(line)
			continue
		}
		if l.ChannelStatus != channels.StatusCancelled {
			s.sumChannel(&out.Totals, l.CollectedBy, gross, commission, net)
		}
		s.compare(&out.Totals, &line, l, l.ChannelStatus, gross, commission)
		add(line)
	}
	if sess != nil {
		s.finish(ctx, sess, runResult{
			status: model.RunSucceeded, total: len(out.Lines), ok: out.Counts[model.MatchOK], failed: len(out.Lines) - out.Counts[model.MatchOK],
			details: map[string]any{"from": out.From, "to": out.To, "counts": out.Counts},
		})
	}
	return out, nil
}

func (s *Service) sumChannel(t *model.ReconciliationTotals, collectedBy string, gross, commission, net money.Money) {
	t.ChannelGross = t.ChannelGross.Add(gross)
	t.ChannelCommission = t.ChannelCommission.Add(commission)
	t.ChannelNet = t.ChannelNet.Add(net)
	if collectedBy == channels.CollectedBySupplier {
		t.CollectedByUs = t.CollectedByUs.Add(commission)
	} else {
		t.CollectedByChannel = t.CollectedByChannel.Add(net)
	}
}

func (s *Service) compareLocalOnly(t *model.ReconciliationTotals, line *model.ReconciliationLine, l db.ListChannelReservationsForReconciliationRow) {
	if l.ReservationUuid.Valid {
		line.Reservation = &model.Ref{UUID: l.ReservationUuid.Bytes, Reference: textOr(l.ReservationReference), Status: textOr(l.ReservationStatus)}
		total, commission := money.FromNumericOrZero(l.ReservationTotal), money.FromNumericOrZero(l.ReservationCommission)
		line.LocalTotal, line.LocalCommission = &total, &commission
		if !terminalStatuses[textOr(l.ReservationStatus)] {
			t.LocalGross = t.LocalGross.Add(total)
			t.LocalCommission = t.LocalCommission.Add(commission)
		}
	}
}

// compare matches one booking: status first, then the total, then the
// commission.
func (s *Service) compare(t *model.ReconciliationTotals, line *model.ReconciliationLine, l db.ListChannelReservationsForReconciliationRow,
	channelStatus string, total, commission money.Money) {
	s.compareLocalOnly(t, line, l)
	cancelledThere := channelStatus == channels.StatusCancelled
	switch {
	case line.Reservation == nil:
		if cancelledThere {
			line.Match = model.MatchOK
		} else {
			line.Match = model.MatchMissingLocal
		}
	case cancelledThere != (textOr(l.ReservationStatus) == "cancelled"):
		line.Match = model.MatchStatus
	case cancelledThere:
		line.Match = model.MatchOK
	case total.IsPositive() && !line.LocalTotal.Equal(total):
		line.Match = model.MatchAmount
		d := total.Sub(*line.LocalTotal)
		line.Difference = &d
	case textOr(l.ReservationCollectingParty) == channels.CollectedByChannel && !line.LocalCommission.Equal(commission):
		line.Match = model.MatchCommission
		d := commission.Sub(*line.LocalCommission)
		line.Difference = &d
	default:
		line.Match = model.MatchOK
	}
}
