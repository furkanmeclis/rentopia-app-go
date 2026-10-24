// Package motomarks is the HTTP client of the motomarks-mirror service: the
// vehicle brand logo API this app syncs brand logos from. The base URL and
// token live in the database (admin-editable), never in env. Only the JSON
// endpoints and the logo files are used; CDN URLs are never handed to
// browsers, the files are copied into this app's own storage.
package motomarks

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Logo variants and the size / format this app copies.
const (
	VariantFull     = "full"
	VariantBadge    = "badge"
	VariantWordmark = "wordmark"
	sizeMD          = "md"
)

// Variants lists every variant in download order.
var Variants = []string{VariantBadge, VariantWordmark, VariantFull}

// MaxBatch is the largest POST /match request the mirror accepts.
const MaxBatch = 500

// MaxLogoBytes caps one downloaded logo file.
const MaxLogoBytes = 2 << 20

var (
	// ErrUnauthorized is a 401 from the mirror (wrong token).
	ErrUnauthorized = errors.New("motomarks: unauthorized")
	// ErrNotConfigured means no base URL / token was set.
	ErrNotConfigured = errors.New("motomarks: not configured")
)

// Brand is one GET /brands item (summary shape).
type Brand struct {
	ID           string                                  `json:"id"`
	Name         string                                  `json:"name"`
	Color        string                                  `json:"color"`
	Colors       []string                                `json:"colors"`
	VehicleTypes []string                                `json:"vehicleTypes"`
	Logo         string                                  `json:"logo"`
	Logos        map[string]map[string]map[string]string `json:"logos"`
}

// LogoURL picks the md webp (else png) file of a variant, "" when absent.
func (b Brand) LogoURL(variant string) string {
	sizes, ok := b.Logos[variant]
	if !ok {
		return ""
	}
	formats, ok := sizes[sizeMD]
	if !ok {
		return ""
	}
	if u := formats["webp"]; u != "" {
		return u
	}
	return formats["png"]
}

// Hash summarizes everything that would make the stored copy stale.
func (b Brand) Hash() string {
	var sb strings.Builder
	sb.WriteString(b.Name)
	sb.WriteString("|")
	sb.WriteString(b.Color)
	for _, v := range Variants {
		sb.WriteString("|")
		sb.WriteString(v)
		sb.WriteString("=")
		sb.WriteString(b.LogoURL(v))
	}
	sum := sha1.Sum([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// MatchResult is one POST /match result.
type MatchResult struct {
	Query   string  `json:"query"`
	Matched bool    `json:"matched"`
	Score   float64 `json:"score"`
	Brand   *Brand  `json:"brand"`
}

// Client talks to one mirror deployment.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	// Requests counts calls made through this client (for run stats).
	Requests int
}

// New builds a client. baseURL is the mirror host without a path prefix.
func New(baseURL, token string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, ErrNotConfigured
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: invalid base url", ErrNotConfigured)
	}
	return &Client{baseURL: baseURL, token: strings.TrimSpace(token), http: &http.Client{Timeout: 45 * time.Second}}, nil
}

// SetHTTPClient swaps the transport (tests).
func (c *Client) SetHTTPClient(h *http.Client) { c.http = h }

func (c *Client) do(ctx context.Context, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Requests++
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("motomarks: %s %s: %w", method, path, err)
	}
	if res.StatusCode == http.StatusUnauthorized {
		_ = res.Body.Close()
		return nil, ErrUnauthorized
	}
	return res, nil
}

// Health checks GET /healthz (no auth) and a token-protected call.
func (c *Client) Health(ctx context.Context) error {
	res, err := c.do(ctx, http.MethodGet, "/healthz", nil, nil)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("motomarks: healthz returned %d", res.StatusCode)
	}
	res2, err := c.do(ctx, http.MethodGet, "/brands?q=__ping__", nil, nil)
	if err != nil {
		return err
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusOK {
		return fmt.Errorf("motomarks: brands returned %d", res2.StatusCode)
	}
	return nil
}

// BrandsResult is GET /brands with ETag handling.
type BrandsResult struct {
	Brands      []Brand
	ETag        string
	NotModified bool
}

// Brands fetches every brand. With a stored etag the mirror answers 304 and
// NotModified is set (Brands empty).
func (c *Client) Brands(ctx context.Context, etag string) (BrandsResult, error) {
	headers := map[string]string{}
	if etag != "" {
		headers["If-None-Match"] = etag
	}
	res, err := c.do(ctx, http.MethodGet, "/brands", nil, headers)
	if err != nil {
		return BrandsResult{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotModified {
		return BrandsResult{ETag: etag, NotModified: true}, nil
	}
	if res.StatusCode != http.StatusOK {
		return BrandsResult{}, fmt.Errorf("motomarks: brands returned %d", res.StatusCode)
	}
	var brands []Brand
	if err := json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(&brands); err != nil {
		return BrandsResult{}, fmt.Errorf("motomarks: decode brands: %w", err)
	}
	sort.Slice(brands, func(i, j int) bool { return brands[i].ID < brands[j].ID })
	return BrandsResult{Brands: brands, ETag: res.Header.Get("ETag")}, nil
}

// Match resolves free-text brand names to mirror brands in batches of
// MaxBatch. results[i] corresponds to queries[i]. minScore 0 returns the
// best candidate for every name so callers can apply their own threshold.
func (c *Client) Match(ctx context.Context, queries []string, minScore float64) ([]MatchResult, error) {
	out := make([]MatchResult, 0, len(queries))
	for start := 0; start < len(queries); start += MaxBatch {
		end := start + MaxBatch
		if end > len(queries) {
			end = len(queries)
		}
		body, err := json.Marshal(map[string]any{"queries": queries[start:end], "minScore": minScore, "limit": 1})
		if err != nil {
			return nil, err
		}
		res, err := c.do(ctx, http.MethodPost, "/match", body, nil)
		if err != nil {
			return nil, err
		}
		var payload struct {
			Results []MatchResult `json:"results"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(&payload)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("motomarks: match returned %d", res.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("motomarks: decode match: %w", decodeErr)
		}
		if len(payload.Results) != end-start {
			return nil, fmt.Errorf("motomarks: match returned %d results for %d queries", len(payload.Results), end-start)
		}
		out = append(out, payload.Results...)
	}
	return out, nil
}

// Download fetches one logo file (a public CDN URL from the brand list).
func (c *Client) Download(ctx context.Context, fileURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, "", err
	}
	c.Requests++
	res, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("motomarks: download: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("motomarks: download returned %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxLogoBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("motomarks: download: %w", err)
	}
	if len(data) > MaxLogoBytes {
		return nil, "", fmt.Errorf("motomarks: logo larger than %d bytes", MaxLogoBytes)
	}
	return data, res.Header.Get("Content-Type"), nil
}
