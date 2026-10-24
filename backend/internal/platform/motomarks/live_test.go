package motomarks

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveMirror talks to a real mirror when MOTOMARKS_TEST_URL and
// MOTOMARKS_TEST_TOKEN are set (skipped in CI).
func TestLiveMirror(t *testing.T) {
	base, token := os.Getenv("MOTOMARKS_TEST_URL"), os.Getenv("MOTOMARKS_TEST_TOKEN")
	if base == "" || token == "" {
		t.Skip("MOTOMARKS_TEST_URL / MOTOMARKS_TEST_TOKEN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := New(base, token)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Health(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := c.Brands(ctx, "")
	if err != nil || len(res.Brands) < 100 || res.ETag == "" {
		t.Fatalf("brands: %v n=%d etag=%q", err, len(res.Brands), res.ETag)
	}
	again, err := c.Brands(ctx, res.ETag)
	if err != nil || !again.NotModified {
		t.Fatalf("etag revalidation: %v %+v", err, again.NotModified)
	}
	var withBadge *Brand
	for i := range res.Brands {
		if res.Brands[i].LogoURL(VariantBadge) != "" {
			withBadge = &res.Brands[i]
			break
		}
	}
	if withBadge == nil {
		t.Fatal("no brand with a badge")
	}
	data, ct, err := c.Download(ctx, withBadge.LogoURL(VariantBadge))
	if err != nil || len(data) == 0 {
		t.Fatalf("download: %v ct=%q n=%d", err, ct, len(data))
	}
	t.Logf("downloaded %s badge: %d bytes (%s)", withBadge.ID, len(data), ct)
	m, err := c.Match(ctx, []string{"Tofaş-Fiat", "Ford /USA", "Temsa"}, 0)
	if err != nil || len(m) != 3 {
		t.Fatalf("match: %v %+v", err, m)
	}
	if m[0].Brand == nil || m[0].Brand.ID != "fiat" || m[1].Brand == nil || m[1].Brand.ID != "ford" {
		t.Fatalf("match results: %+v", m)
	}
	t.Logf("requests made: %d", c.Requests)
}
