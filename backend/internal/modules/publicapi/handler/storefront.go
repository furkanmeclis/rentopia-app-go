package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	campaignusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	sitemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	leadsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/model"
	leadsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Public) SetStorefront(q *db.Queries, leads *leadsusecase.Service) {
	h.storefrontQ = q
	h.leads = leads
}
func (h *Public) storefront(w http.ResponseWriter, r *http.Request) bool {
	c := credential(r)
	if !c.Storefront() || c.SessionID == "" || c.Channel != "web" {
		response.Forbidden(w, r, "Storefront session required")
		return false
	}
	return true
}
func (h *Public) Campaigns(w http.ResponseWriter, r *http.Request) { h.campaigns(w, r, nil) }
func (h *Public) Campaign(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	h.campaigns(w, r, &id)
}
func (h *Public) campaigns(w http.ResponseWriter, r *http.Request, id *uuid.UUID) {
	if !h.storefront(w, r) {
		return
	}
	if h.storefrontQ == nil {
		response.Internal(w, r, "Storefront unavailable")
		return
	}
	c := credential(r)
	out, err := campaignusecase.PublicCampaigns(r.Context(), h.storefrontQ, c.OrganizationID, c.Channel, id)
	if err != nil {
		response.InternalErr(w, r, err, "Campaigns unavailable")
		return
	}
	if id != nil {
		if len(out) == 0 {
			response.NotFound(w, r, "Campaign not found")
			return
		}
		response.JSON(w, r, http.StatusOK, out[0])
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Public) SubmitLead(w http.ResponseWriter, r *http.Request) {
	if !h.storefront(w, r) {
		return
	}
	if h.leads == nil || h.storefrontQ == nil {
		response.Internal(w, r, "Site leads unavailable")
		return
	}
	c := credential(r)
	ip := httpkit.ClientIP(r)
	subject := strconv.FormatInt(c.OrganizationID, 10) + ":" + h.leads.IPHash(c.OrganizationID, ip)
	if !h.allow(w, r, "siteleads:ip", subject, 10, time.Hour) {
		return
	}
	if !h.allow(w, r, "siteleads:session", c.SessionID, 5, time.Hour) {
		return
	}
	var in leadsmodel.Input
	if !decode(w, r, &in) {
		return
	}
	if in.Honeypot != "" {
		response.JSON(w, r, http.StatusAccepted, map[string]bool{"accepted": true})
		return
	}
	if err := leadsusecase.Validate(&in); err != nil {
		var ve *leadsusecase.ValidationError
		if errors.As(err, &ve) {
			response.ValidationError(w, r, ve.Details)
		}
		return
	}
	site, err := h.storefrontQ.GetSiteSettings(r.Context(), c.OrganizationID)
	if err != nil || !site.Enabled || !site.PublishedRevision.Valid || !site.ApiKeyID.Valid || site.ApiKeyID.Int64 != c.KeyID {
		response.NotFound(w, r, "Site form not found")
		return
	}
	var cfg sitemodel.Config
	if err := json.Unmarshal(site.PublishedConfig, &cfg); err != nil {
		response.Internal(w, r, "Site forms unavailable")
		return
	}
	if !cfg.Leads.Enabled(in.Kind) {
		response.NotFound(w, r, "Site form not found")
		return
	}
	if err := h.leads.Submit(r.Context(), c.OrganizationID, in, c.Origin, ip, r.UserAgent()); err != nil {
		response.InternalErr(w, r, err, "Site form unavailable")
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]bool{"accepted": true})
}
