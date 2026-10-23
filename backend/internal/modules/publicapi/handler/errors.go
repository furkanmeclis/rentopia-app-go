package handler

import (
	"errors"
	"net/http"
	"strings"

	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	portalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Error codes of the public API (in addition to the shared envelope codes).
const (
	CodeRateLimited            = "RATE_LIMITED"
	CodeOriginNotAllowed       = "ORIGIN_NOT_ALLOWED"
	CodeSecretKeyInBrowser     = "SECRET_KEY_IN_BROWSER"
	CodeScope                  = "INSUFFICIENT_SCOPE"
	CodeChannelDisabled        = "CHANNEL_DISABLED"
	CodeOrganizationUnavail    = "ORGANIZATION_UNAVAILABLE"
	CodeIdempotencyRequired    = "IDEMPOTENCY_KEY_REQUIRED"
	CodeIdempotencyInProgress  = "IDEMPOTENCY_IN_PROGRESS"
	CodeIdempotencyKeyReused   = "IDEMPOTENCY_KEY_REUSED"
	CodeInvalidState           = "INVALID_STATE"
	CodePricing                = "PRICING_UNAVAILABLE"
	CodeDriverNotEligible      = "DRIVER_NOT_ELIGIBLE"
	CodePaymentUnavailable     = "PAYMENT_UNAVAILABLE"
	CodeNothingDue             = "NOTHING_DUE"
	CodeRequestExists          = "REQUEST_EXISTS"
	CodeKeyRevoked             = "KEY_REVOKED"
	CodeQuotePriceChangedField = "quote_uuid"
)

func details[T any](list []T, conv func(T) response.Detail) []response.Detail {
	out := make([]response.Detail, 0, len(list))
	for _, d := range list {
		out = append(out, conv(d))
	}
	return out
}

// publicAlternatives hides capacity numbers beyond the public cap.
func publicAlternatives(ce *availusecase.CapacityError) map[string]any {
	products := []map[string]any{}
	times := []map[string]any{}
	if ce.Alternatives != nil {
		for _, p := range ce.Alternatives.Products {
			products = append(products, map[string]any{
				"subgroup":   model.SubgroupRef{UUID: p.Product.UUID, Code: p.Product.Code, Title: p.Product.Title},
				"units_left": min(p.Sellable, model.MaxUnitsLeft),
			})
		}
		for _, t := range ce.Alternatives.Times {
			times = append(times, map[string]any{"starts_at": t.StartsAt, "ends_at": t.EndsAt, "units_left": min(t.Sellable, model.MaxUnitsLeft)})
		}
	}
	return map[string]any{"subgroups": products, "times": times}
}

// writeError maps every use case error of the public API onto the
// envelope. Internal details never leave: unexpected errors are 500.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if features.WriteError(w, r, err) {
		return
	}
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, details(ve.Details, func(d usecase.Detail) response.Detail {
			return response.Detail{Field: d.Field, Code: d.Code, Message: d.Message}
		}))
		return
	}
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		response.ValidationError(w, r, details(qve.Details, func(d apiquery.Detail) response.Detail {
			return response.Detail{Field: d.Field, Code: d.Code, Message: d.Message}
		}))
		return
	}
	var rve *resusecase.ValidationError
	if errors.As(err, &rve) {
		response.ValidationError(w, r, details(rve.Details, func(d resusecase.Detail) response.Detail {
			return response.Detail{Field: d.Field, Code: d.Code, Message: d.Message}
		}))
		return
	}
	var cve *customerusecase.ValidationError
	if errors.As(err, &cve) {
		response.ValidationError(w, r, details(cve.Details, func(d customerusecase.Detail) response.Detail {
			return response.Detail{Field: "customer." + d.Field, Code: d.Code, Message: d.Message}
		}))
		return
	}
	var pve *portalusecase.ValidationError
	if errors.As(err, &pve) {
		response.ValidationError(w, r, details(pve.Details, func(d portalusecase.Detail) response.Detail {
			return response.Detail{Field: d.Field, Code: d.Code, Message: d.Message}
		}))
		return
	}
	if ce, ok := availusecase.IsCapacity(err); ok {
		response.ErrorWithData(w, r, http.StatusConflict, response.CodeCapacityUnavailable,
			"No vehicle of this subgroup is available for the whole range",
			[]response.Detail{{Code: ce.Reason, Message: ce.Reason}}, publicAlternatives(ce))
		return
	}
	var qr *usecase.QuoteReplacedError
	if errors.As(err, &qr) {
		response.ErrorWithData(w, r, http.StatusConflict, qr.Err.Code, qr.Err.Message,
			[]response.Detail{{Field: "quote_uuid", Code: strings.ToLower(qr.Err.Code), Message: qr.Err.Message}}, qr.Replacement)
		return
	}
	var qe *resusecase.QuoteError
	if errors.As(err, &qe) {
		response.ErrorWithDetails(w, r, http.StatusConflict, qe.Code, qe.Message,
			[]response.Detail{{Field: "quote_uuid", Code: strings.ToLower(qe.Code), Message: qe.Message}})
		return
	}
	var pe *resusecase.PricingError
	if errors.As(err, &pe) {
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodePricing, pe.Message, []response.Detail{{Code: pe.Code, Message: pe.Message}})
		return
	}
	var ee *resusecase.EligibilityError
	if errors.As(err, &ee) {
		out := []response.Detail{}
		for i, d := range ee.Drivers {
			for _, reason := range d.Reasons {
				out = append(out, response.Detail{Field: "drivers." + strings.TrimSpace(itoa(i)), Code: reason, Message: reason})
			}
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeDriverNotEligible, ee.Error(), out)
		return
	}
	var eq *resmodel.EquipmentError
	if errors.As(err, &eq) {
		response.ErrorWithData(w, r, http.StatusConflict, response.CodeEquipmentUnavailable, "An extra is out of stock for these dates",
			[]response.Detail{{Field: "extras", Code: "equipment_unavailable", Message: eq.ExtraUUID.String()}},
			map[string]any{"extra_uuid": eq.ExtraUUID})
		return
	}
	var rse *resusecase.StateError
	if errors.As(err, &rse) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, rse.Message, []response.Detail{{Code: rse.Code, Message: rse.Message}})
		return
	}
	var pse *portalusecase.StateError
	if errors.As(err, &pse) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, pse.Message, []response.Detail{{Code: pse.Code, Message: pse.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrUnauthenticated):
		response.Unauthorized(w, r, err.Error())
	case errors.Is(err, usecase.ErrOriginNotAllowed):
		response.ErrorWithDetails(w, r, http.StatusForbidden, CodeOriginNotAllowed, err.Error(), []response.Detail{{Code: "origin_not_allowed", Message: err.Error()}})
	case errors.Is(err, usecase.ErrSecretKeyInBrowser):
		response.ErrorWithDetails(w, r, http.StatusForbidden, CodeSecretKeyInBrowser, err.Error(), []response.Detail{{Code: "secret_key_in_browser", Message: err.Error()}})
	case errors.Is(err, usecase.ErrScope):
		response.ErrorWithDetails(w, r, http.StatusForbidden, CodeScope, err.Error(), []response.Detail{{Code: "insufficient_scope", Message: err.Error()}})
	case errors.Is(err, usecase.ErrChannelDisabled):
		response.ErrorWithDetails(w, r, http.StatusForbidden, CodeChannelDisabled, err.Error(), []response.Detail{{Code: "channel_disabled", Message: err.Error()}})
	case errors.Is(err, usecase.ErrOrganizationUnavailable):
		response.ErrorWithDetails(w, r, http.StatusForbidden, CodeOrganizationUnavail, err.Error(), []response.Detail{{Code: "organization_unavailable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrIdempotencyKeyRequired):
		response.ErrorWithDetails(w, r, http.StatusBadRequest, CodeIdempotencyRequired, err.Error(), []response.Detail{{Field: HeaderIdempotencyKey, Code: "required", Message: err.Error()}})
	case errors.Is(err, usecase.ErrIdempotencyInProgress):
		w.Header().Set("Retry-After", "2")
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeIdempotencyInProgress, err.Error(), []response.Detail{{Field: HeaderIdempotencyKey, Code: "in_progress", Message: err.Error()}})
	case errors.Is(err, usecase.ErrIdempotencyMismatch):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeIdempotencyKeyReused, err.Error(), []response.Detail{{Field: HeaderIdempotencyKey, Code: "reused", Message: err.Error()}})
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, usecase.ErrReservationToken),
		errors.Is(err, portalusecase.ErrNotFound), errors.Is(err, portalusecase.ErrPortalNotFound),
		errors.Is(err, resusecase.ErrNotFound), errors.Is(err, resusecase.ErrQuoteNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrKeyNotActive):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeKeyRevoked, err.Error(), []response.Detail{{Code: "key_revoked", Message: err.Error()}})
	case errors.Is(err, portalusecase.ErrPaymentUnavailable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodePaymentUnavailable, err.Error(), []response.Detail{{Code: "payment_unavailable", Message: err.Error()}})
	case errors.Is(err, portalusecase.ErrNothingDue):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNothingDue, err.Error(), []response.Detail{{Code: "nothing_due", Message: err.Error()}})
	case errors.Is(err, portalusecase.ErrRequestExists):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeRequestExists, err.Error(), []response.Detail{{Code: "request_exists", Message: err.Error()}})
	case errors.Is(err, resusecase.ErrExternalReferenceTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Field: "external_reference", Code: "taken", Message: err.Error()}})
	case errors.Is(err, resusecase.ErrForbidden):
		response.Forbidden(w, r, "This action is not available through the public API")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func itoa(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return digits[i : i+1]
	}
	return itoa(i/10) + digits[i%10:i%10+1]
}
