package adapters

import (
	"context"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
)

func TestImportSchemaMapsIdesHeaders(t *testing.T) {
	t.Parallel()
	headers := []string{"Ürün kodu", "Bant", "Günlük fiyat (TRY)", "Şube kodu", "Kanal", "Öncelik"}
	got := ioengine.SuggestMapping(headers, NewRatePlansIO(nil).ImportSchema(), i18n.Normalize("tr"))
	want := map[string]string{
		"Ürün kodu": "product_code", "Bant": "band", "Günlük fiyat (TRY)": "daily_price",
		"Şube kodu": "branch_code", "Kanal": "channel", "Öncelik": "priority",
	}
	for h, key := range want {
		if got[h] != key {
			t.Errorf("header %q → %q, want %q (mapping %v)", h, got[h], key, got)
		}
	}
}

func TestAdaptersNeedAnOrganization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := NewRatePlansIO(nil).ApplyRow(ctx, map[string]any{"product_code": "X"}, nil); err == nil {
		t.Fatal("import without an organization must fail")
	}
	if _, err := NewRatePlansIO(nil).Export(ctx, nil, i18n.Normalize("tr")); err == nil {
		t.Fatal("export without an organization must fail")
	}
	if _, err := NewDateRulesIO(nil).Export(ctx, nil, i18n.Normalize("tr")); err == nil {
		t.Fatal("rule export without an organization must fail")
	}
	if len(NewDateRulesIO(nil).ImportSchema()) != 0 {
		t.Fatal("date rules are export-only")
	}
}
