package motomarks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrandsETagAndLogoURL(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_ = json.NewEncoder(w).Encode([]Brand{{
			ID: "audi", Name: "Audi", Color: "#000",
			Logos: map[string]map[string]map[string]string{
				"full":  {"md": {"png": "http://cdn/full.png"}},
				"badge": {"md": {"webp": "http://cdn/badge.webp", "png": "http://cdn/badge.png"}},
			},
		}})
	}))
	defer srv.Close()

	c, err := New(srv.URL+"/", "tok")
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Brands(context.Background(), "")
	if err != nil || res.NotModified || len(res.Brands) != 1 || res.ETag != `"v1"` {
		t.Fatalf("brands: %v %+v", err, res)
	}
	b := res.Brands[0]
	if b.LogoURL(VariantBadge) != "http://cdn/badge.webp" || b.LogoURL(VariantFull) != "http://cdn/full.png" || b.LogoURL(VariantWordmark) != "" {
		t.Fatalf("logo urls: %q %q %q", b.LogoURL(VariantBadge), b.LogoURL(VariantFull), b.LogoURL(VariantWordmark))
	}
	hash := b.Hash()
	if hash == "" || hash != b.Hash() {
		t.Fatal("hash must be stable")
	}
	again, err := c.Brands(context.Background(), res.ETag)
	if err != nil || !again.NotModified || again.ETag != `"v1"` {
		t.Fatalf("304: %v %+v", err, again)
	}
	bad, _ := New(srv.URL, "wrong")
	if _, err := bad.Brands(context.Background(), ""); err != ErrUnauthorized {
		t.Fatalf("unauthorized = %v", err)
	}
	if calls != 3 || c.Requests != 2 {
		t.Fatalf("calls=%d requests=%d", calls, c.Requests)
	}
}

func TestMatchBatches(t *testing.T) {
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Queries []string `json:"queries"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		sizes = append(sizes, len(in.Queries))
		out := make([]MatchResult, 0, len(in.Queries))
		for _, q := range in.Queries {
			out = append(out, MatchResult{Query: q, Matched: q == "Fiat", Score: 100, Brand: &Brand{ID: "fiat"}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "")
	queries := make([]string, 0, MaxBatch+3)
	for i := 0; i < MaxBatch+2; i++ {
		queries = append(queries, "x")
	}
	queries = append(queries, "Fiat")
	res, err := c.Match(context.Background(), queries, 0)
	if err != nil || len(res) != len(queries) {
		t.Fatalf("match: %v %d", err, len(res))
	}
	if len(sizes) != 2 || sizes[0] != MaxBatch || sizes[1] != 3 {
		t.Fatalf("batches = %v", sizes)
	}
	if !res[len(res)-1].Matched || res[len(res)-1].Brand.ID != "fiat" {
		t.Fatalf("last = %+v", res[len(res)-1])
	}
	if _, err := New("", "t"); err == nil {
		t.Fatal("empty base url must fail")
	}
	if _, err := New("ftp://x", "t"); err == nil {
		t.Fatal("bad scheme must fail")
	}
}
