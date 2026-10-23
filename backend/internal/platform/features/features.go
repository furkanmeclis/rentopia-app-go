// Package features holds the plan feature catalog and the entitlement
// rules of TEC-674 (RT-A28).
//
// An organization has one plan (plans: management = Yönetim, complete =
// Komple) and may carry per-feature overrides. The effective state of a
// feature is the override when there is one, else the plan default
// (Effective). The panel itself (customers, fleet, reservations, rentals,
// contracts, finance, documents, customer portal links, e-mail
// notifications) is never a feature: every plan has it.
//
// Enforcement is server side: tenant routes carry
// middleware.RequireFeature (the resolved orgctx.Scope holds the enabled
// keys), public and background paths ask an AccessPolicy. A feature that
// is off answers 403 FEATURE_NOT_ENABLED and never deletes data.
//
// The catalog must match the features seeded by migration 000134 and
// frontend/src/config/features.ts (catalog_test.go).
package features

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Feature is a feature key.
type Feature string

// Feature keys.
const (
	Site            Feature = "site"
	Widget          Feature = "widget"
	PublicAPI       Feature = "public_api"
	ChannelYolcu360 Feature = "channel.yolcu360"
	ChannelObilet   Feature = "channel.obilet"
	Kabis           Feature = "kabis"
	HGS             Feature = "hgs"
	EInvoice        Feature = "einvoice"
	WhatsApp        Feature = "whatsapp"
	SMS             Feature = "sms"
)

// Plan codes seeded by migration 000134.
const (
	PlanManagement = "management"
	PlanComplete   = "complete"
)

// DefaultPlan is the plan of new organizations (unless the platform picks
// another) and of an organization without a plan row.
const DefaultPlan = PlanComplete

// Info describes a feature for screens and the platform API.
type Info struct {
	Key    Feature `json:"key"`
	NameTR string  `json:"name_tr"`
	NameEN string  `json:"name_en"`
}

// catalog lists every feature in display order.
var catalog = []Info{
	{Site, "Markalı web sitesi", "Branded website"},
	{Widget, "Rezervasyon widget'ı", "Booking widget"},
	{PublicAPI, "Açık API", "Public API"},
	{ChannelYolcu360, "Yolcu360 kanalı", "Yolcu360 channel"},
	{ChannelObilet, "Obilet kanalı", "Obilet channel"},
	{Kabis, "KABİS bildirimleri", "KABİS notifications"},
	{HGS, "HGS / geçiş ücretleri", "HGS / toll passes"},
	{EInvoice, "e-Fatura / e-Arşiv gönderimi", "e-Invoice / e-Archive sending"},
	{WhatsApp, "WhatsApp", "WhatsApp"},
	{SMS, "SMS", "SMS"},
}

// Catalog returns the features in display order.
func Catalog() []Info { return append([]Info(nil), catalog...) }

// All returns every feature key in display order.
func All() []Feature {
	out := make([]Feature, 0, len(catalog))
	for _, f := range catalog {
		out = append(out, f.Key)
	}
	return out
}

// AllStrings returns every feature key as strings (a scope without a
// feature source: everything is on, like the default plan).
func AllStrings() []string { return Strings(All()) }

// Strings converts keys to strings.
func Strings(in []Feature) []string {
	out := make([]string, 0, len(in))
	for _, f := range in {
		out = append(out, string(f))
	}
	return out
}

// Parse returns the feature of a key (false for an unknown key).
func Parse(key string) (Feature, bool) {
	key = strings.TrimSpace(key)
	for _, f := range catalog {
		if string(f.Key) == key {
			return f.Key, true
		}
	}
	return "", false
}

// Valid reports whether key is a known feature.
func Valid(key string) bool {
	_, ok := Parse(key)
	return ok
}

// State is one feature of an organization: its plan default and override.
type State struct {
	Key    Feature
	InPlan bool
	// Override is nil without an organization override.
	Override *bool
}

// Enabled is the effective state: the override beats the plan default.
func (s State) Enabled() bool {
	if s.Override != nil {
		return *s.Override
	}
	return s.InPlan
}

// Effective returns the enabled features of states, in input order;
// unknown keys are dropped.
func Effective(states []State) []Feature {
	out := make([]Feature, 0, len(states))
	for _, s := range states {
		if Valid(string(s.Key)) && s.Enabled() {
			out = append(out, s.Key)
		}
	}
	return out
}

// Contains reports whether keys holds f.
func Contains(keys []string, f Feature) bool {
	for _, k := range keys {
		if k == string(f) {
			return true
		}
	}
	return false
}

// ErrNotEnabled: the feature is not in the organization's plan (or is
// switched off for it). HTTP: 403 FEATURE_NOT_ENABLED.
var ErrNotEnabled = errors.New("feature is not enabled for the organization")

// NotEnabledError names the feature that is off; errors.Is(err,
// ErrNotEnabled) holds.
type NotEnabledError struct{ Feature Feature }

func (e *NotEnabledError) Error() string {
	return fmt.Sprintf("the %q feature is not included in the organization's plan", string(e.Feature))
}

// Is makes errors.Is(err, ErrNotEnabled) true.
func (e *NotEnabledError) Is(target error) bool { return target == ErrNotEnabled }

// NotEnabled builds the error of an off feature.
func NotEnabled(f Feature) error { return &NotEnabledError{Feature: f} }

// FeatureOf returns the feature named by a NotEnabledError in err.
func FeatureOf(err error) (Feature, bool) {
	var ne *NotEnabledError
	if errors.As(err, &ne) {
		return ne.Feature, true
	}
	return "", false
}

// ------------------------------------------------------------- mappings

// ForIntegration maps an integration account (kind, provider) to its
// feature. Payment providers (and anything unknown) map to none: payment
// links are never gated.
func ForIntegration(kind, provider string) (Feature, bool) {
	switch strings.TrimSpace(kind) {
	case "sms":
		return SMS, true
	case "kabis":
		return Kabis, true
	case "toll":
		return HGS, true
	case "einvoice":
		return EInvoice, true
	case "whatsapp":
		return WhatsApp, true
	case "channel":
		return ForSalesChannel(strings.TrimSuffix(strings.TrimSpace(provider), "_fake"))
	}
	return "", false
}

// ForProvider maps an integration provider code (integration jobs carry
// the provider, not the account kind) to its feature. SMS providers map to
// none on purpose: SMS jobs also carry the one-time codes of customer
// links, which every plan keeps.
func ForProvider(provider string) (Feature, bool) {
	switch strings.TrimSuffix(strings.TrimSpace(provider), "_fake") {
	case "kabis":
		return Kabis, true
	case "hgs":
		return HGS, true
	case "einvoice":
		return EInvoice, true
	case "wuzapi", "whatsapp":
		return WhatsApp, true
	case "yolcu360":
		return ChannelYolcu360, true
	case "obilet":
		return ChannelObilet, true
	}
	return "", false
}

// ForSalesChannel maps a sales channel code (orgsettings sales_channels,
// public API key channels) to its feature: web = the branded site, widget,
// api = the public API, and the channel managers. The branch counter has
// no feature.
func ForSalesChannel(code string) (Feature, bool) {
	switch strings.TrimSpace(code) {
	case "web":
		return Site, true
	case "widget":
		return Widget, true
	case "api":
		return PublicAPI, true
	case "yolcu360":
		return ChannelYolcu360, true
	case "obilet":
		return ChannelObilet, true
	}
	return "", false
}

// ForNotifyChannel maps a notification engine channel to its feature.
// E-mail and push are never gated.
func ForNotifyChannel(name string) (Feature, bool) {
	switch strings.TrimSpace(name) {
	case "sms":
		return SMS, true
	case "whatsapp":
		return WhatsApp, true
	}
	return "", false
}

// ------------------------------------------------------------- policy

// AccessPolicy answers whether a feature is enabled for an organization:
// the hook public endpoints (organization resolved from a host, slug or
// API key) and background work (notification sends, channel syncs) ask.
// Implementations cache per organization (orgcache.Features).
type AccessPolicy interface {
	Enabled(ctx context.Context, organizationID int64, f Feature) (bool, error)
}

// Loader returns the enabled features of an organization.
type Loader interface {
	EnabledFeatures(ctx context.Context, organizationID int64) ([]Feature, error)
}

// Source loads an organization's enabled features and its plan
// (features.Store; orgcache.Features caches both per organization).
type Source interface {
	Loader
	Plan(ctx context.Context, organizationID int64) (PlanRef, error)
}

// LoaderPolicy is an uncached AccessPolicy over a Loader.
type LoaderPolicy struct{ Loader Loader }

// Enabled implements AccessPolicy.
func (p LoaderPolicy) Enabled(ctx context.Context, organizationID int64, f Feature) (bool, error) {
	keys, err := p.Loader.EnabledFeatures(ctx, organizationID)
	if err != nil {
		return false, err
	}
	for _, k := range keys {
		if k == f {
			return true, nil
		}
	}
	return false, nil
}

// Static is a fixed AccessPolicy (tests, tools): the listed features are
// on for every organization.
type Static []Feature

// Enabled implements AccessPolicy.
func (s Static) Enabled(_ context.Context, _ int64, f Feature) (bool, error) {
	for _, k := range s {
		if k == f {
			return true, nil
		}
	}
	return false, nil
}

// Require returns NotEnabled(f) when the feature is off for the
// organization. A request scope of the same organization (tenant routes)
// answers from orgctx.Scope.Features without a lookup; otherwise the
// policy is asked. A nil policy allows everything (tenancy off, tests).
func Require(ctx context.Context, p AccessPolicy, organizationID int64, f Feature) error {
	if s, ok := orgctx.From(ctx); ok && s.ID == organizationID && s.Features != nil {
		if s.HasFeature(string(f)) {
			return nil
		}
		return NotEnabled(f)
	}
	if p == nil {
		return nil
	}
	ok, err := p.Enabled(ctx, organizationID, f)
	if err != nil {
		return err
	}
	if !ok {
		return NotEnabled(f)
	}
	return nil
}
