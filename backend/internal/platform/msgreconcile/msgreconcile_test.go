package msgreconcile

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	t.Parallel()
	want := time.Date(2026, 10, 20, 3, 15, 0, 0, time.UTC)
	for _, v := range []string{"2026-10-20T03:15:00Z", "2026-10-20T06:15:00+03:00", "20261020T031500Z"} {
		got, err := ParseTime(v)
		if err != nil || !got.Equal(want) {
			t.Errorf("ParseTime(%q) = %v, %v", v, got, err)
		}
	}
	if _, err := ParseTime("20.10.2026"); err == nil {
		t.Error("an unknown format must fail")
	}
	if got, err := ParseTime(""); err != nil || !got.IsZero() {
		t.Errorf("empty = %v, %v", got, err)
	}
}

func TestFilterValidate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var f Filter
	if err := f.Validate(now); err == nil {
		t.Fatal("the backup time is required")
	}
	f.Before = now.Add(time.Hour)
	if err := f.Validate(now); err == nil {
		t.Fatal("a backup time in the future must fail")
	}
	f.Before = now.Add(-48 * time.Hour)
	if err := f.Validate(now); err != nil {
		t.Fatal(err)
	}
	if !f.DueUntil.Equal(now) || f.Limit != 1000 {
		t.Fatalf("defaults = %+v", f)
	}
	if !f.has(KindSMS) {
		t.Fatal("no kinds = every kind")
	}
	f.Kinds = []Kind{KindDispatch}
	if f.has(KindSMS) || !f.has(KindDispatch) {
		t.Fatal("kinds filter")
	}
}

func TestParseKinds(t *testing.T) {
	t.Parallel()
	k, err := ParseKinds(" dispatch, notification ,")
	if err != nil || len(k) != 2 || k[0] != KindDispatch || k[1] != KindNotification {
		t.Fatalf("kinds = %v, %v", k, err)
	}
	if _, err := ParseKinds("email"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestMaskRecipient(t *testing.T) {
	t.Parallel()
	cases := map[string]string{"": "", "ayse@example.test": "a***@example.test", "+905321112233": "***33", "x": "***"}
	for in, want := range cases {
		if got := MaskRecipient(in); got != want {
			t.Errorf("MaskRecipient(%q) = %q, want %q", in, got, want)
		}
	}
}
