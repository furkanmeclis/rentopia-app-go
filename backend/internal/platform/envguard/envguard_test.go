package envguard

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeAndValid(t *testing.T) {
	cases := map[string]bool{"": true, "development": true, " Staging ": true, "PRODUCTION": true, "test": true, "prod": false, "stage": false}
	for in, want := range cases {
		if got := Valid(in); got != want {
			t.Errorf("Valid(%q) = %v, want %v", in, got, want)
		}
	}
	if Normalize("") != Development {
		t.Fatal("empty APP_ENV must mean development")
	}
	if !IsProduction(" Production") || IsProduction("staging") {
		t.Fatal("IsProduction")
	}
}

func TestRefusal(t *testing.T) {
	if err := Refusal("production", "kabis", AllowKabisKey, false); err != nil {
		t.Fatalf("production must pass: %v", err)
	}
	if err := Refusal("staging", "kabis", AllowKabisKey, true); err != nil {
		t.Fatalf("explicit allow must pass: %v", err)
	}
	for _, env := range []string{"", "development", "staging", "test"} {
		err := Refusal(env, "kabis", AllowKabisKey, false)
		if !errors.Is(err, ErrProductionRefused) {
			t.Fatalf("%q: want refusal, got %v", env, err)
		}
		if !strings.Contains(err.Error(), AllowKabisKey) {
			t.Fatalf("message must name the allow key: %v", err)
		}
	}
}
