package usecase

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
)

func testService(t *testing.T) *Service {
	t.Helper()
	c, err := crypto.NewFieldCipher(strings.Repeat("u", 32))
	if err != nil {
		t.Fatal(err)
	}
	return New(nil, c, nil, nil, nil)
}

func detailCodes(err error) map[string]string {
	out := map[string]string{}
	var ve *ValidationError
	if errors.As(err, &ve) {
		for _, d := range ve.Details {
			out[d.Field] = d.Code
		}
	}
	return out
}

func TestApplyCustomerEncryptsIdentity(t *testing.T) {
	s := testService(t)
	f := repository.CustomerFields{CustomerType: model.TypeIndividual, Tags: []string{}}
	in := model.CustomerInput{
		FirstName: model.Str("Ayşe"), LastName: model.Str("Yılmaz"), IdentityType: model.String("tckn"),
		IdentityNumber: model.Str("100 000 001 46"), Phone: model.Str("0532 123 45 12"), Email: model.Str(" A@B.COM "),
		Tags: &[]string{"VIP", "vip", " Filo "},
	}
	if err := s.applyCustomer(7, &f, in, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.IdentityEnc, "10000000146") || f.IdentityHint != "100******46" || len(f.IdentityBidx) != 64 {
		t.Fatalf("identity fields: %+v", f)
	}
	if plain, err := s.open(7, colCustomerIdentity, f.IdentityEnc); err != nil || plain != "10000000146" {
		t.Fatalf("decrypt = %q, %v", plain, err)
	}
	if _, err := s.open(8, colCustomerIdentity, f.IdentityEnc); err == nil {
		t.Fatal("another organization opened the identity ciphertext")
	}
	if f.IdentityBidx != s.identityIndex(7, model.IdentityTCKN, "10000000146") {
		t.Fatal("blind index differs from the lookup index")
	}
	if f.Phone != "+905321234512" || f.Email != "a@b.com" {
		t.Fatalf("contact normalization: %q %q", f.Phone, f.Email)
	}
	if strings.Join(f.Tags, "|") != "VIP|Filo" {
		t.Fatalf("tags = %v", f.Tags)
	}
	// An empty identity number clears the identity.
	if err := s.applyCustomer(7, &f, model.CustomerInput{IdentityNumber: model.Str("")}, false); err != nil {
		t.Fatal(err)
	}
	if f.IdentityType.Valid || f.IdentityEnc != "" || f.IdentityBidx != "" || f.IdentityHint != "" {
		t.Fatalf("identity not cleared: %+v", f)
	}
}

func TestApplyCustomerValidation(t *testing.T) {
	s := testService(t)
	cases := []struct {
		name string
		in   model.CustomerInput
		want map[string]string
	}{
		{"individual without names", model.CustomerInput{CustomerType: model.Str("individual")},
			map[string]string{"first_name": "required", "last_name": "required"}},
		{"corporate without title", model.CustomerInput{CustomerType: model.Str("corporate")},
			map[string]string{"company_title": "required"}},
		{"bad tckn checksum", model.CustomerInput{FirstName: model.Str("a"), LastName: model.Str("b"),
			IdentityType: model.String("tckn"), IdentityNumber: model.Str("10000000147")},
			map[string]string{"identity_number": "invalid_checksum"}},
		{"vkn on an individual", model.CustomerInput{FirstName: model.Str("a"), LastName: model.Str("b"),
			IdentityType: model.String("vkn"), IdentityNumber: model.Str("4540536920")},
			map[string]string{"identity_type": "invalid"}},
		{"passport needs nationality", model.CustomerInput{FirstName: model.Str("a"), LastName: model.Str("b"),
			IdentityType: model.String("passport"), IdentityNumber: model.Str("U1234567")},
			map[string]string{"nationality": "required"}},
		{"number without type", model.CustomerInput{FirstName: model.Str("a"), LastName: model.Str("b"),
			IdentityNumber: model.Str("10000000146")}, map[string]string{"identity_type": "required"}},
		{"bad phone and email", model.CustomerInput{FirstName: model.Str("a"), LastName: model.Str("b"),
			Phone: model.Str("0532 12"), Email: model.Str("nope")}, map[string]string{"phone": "invalid", "email": "invalid"}},
		{"bad country", model.CustomerInput{FirstName: model.Str("a"), LastName: model.Str("b"),
			Nationality: model.String("TUR")}, map[string]string{"nationality": "invalid"}},
	}
	for _, c := range cases {
		f := repository.CustomerFields{CustomerType: model.TypeIndividual, Tags: []string{}}
		got := detailCodes(s.applyCustomer(1, &f, c.in, true))
		for field, code := range c.want {
			if got[field] != code {
				t.Errorf("%s: %s = %q, want %q (all: %v)", c.name, field, got[field], code, got)
			}
		}
	}
	// Corporate customers take a VKN.
	f := repository.CustomerFields{CustomerType: model.TypeIndividual, Tags: []string{}}
	if err := s.applyCustomer(1, &f, model.CustomerInput{
		CustomerType: model.Str("corporate"), CompanyTitle: model.Str("Örnek A.Ş."), IdentityType: model.String("vkn"),
		IdentityNumber: model.Str("4540536920"),
	}, true); err != nil || f.IdentityHint != "454*****20" {
		t.Fatalf("corporate vkn: %v %+v", err, f)
	}
}

func TestQueryHelpers(t *testing.T) {
	s := testService(t)
	got := s.identityCandidates(3, "100 000 001 46")
	if len(got) != 1 || got[0] != s.identityIndex(3, model.IdentityTCKN, "10000000146") {
		t.Fatalf("tckn candidates = %v", got)
	}
	if got := s.identityCandidates(3, "4540536920"); len(got) != 1 || got[0] != s.identityIndex(3, model.IdentityVKN, "4540536920") {
		t.Fatalf("vkn candidates = %v", got)
	}
	if got := s.identityCandidates(3, "u1234567"); len(got) != 1 || got[0] != s.identityIndex(3, model.IdentityPassport, "U1234567") {
		t.Fatalf("passport candidates = %v", got)
	}
	if got := s.identityCandidates(3, "12345"); len(got) != 0 {
		t.Fatalf("partial number candidates = %v", got)
	}
	for in, want := range map[string]string{"0532 123": "532123", "+90 (532)": "90532", "ab12": "", "12": ""} {
		if got := digitsQuery(in); got != want {
			t.Errorf("digitsQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImportDiffIsMasked(t *testing.T) {
	s := testService(t)
	a := repository.CustomerFields{CustomerType: model.TypeIndividual, Tags: []string{}}
	b := a
	if err := s.applyCustomer(1, &b, model.CustomerInput{
		FirstName: model.Str("a"), LastName: model.Str("b"), IdentityType: model.String("tckn"),
		IdentityNumber: model.Str("10000000146"), Phone: model.Str("05321234512"), Email: model.Str("ayse@x.com"),
	}, true); err != nil {
		t.Fatal(err)
	}
	for _, c := range diffFields(a, b, true) {
		for _, v := range []string{c.From, c.To} {
			if strings.Contains(v, "10000000146") || strings.Contains(v, "1234512") || strings.Contains(v, "ayse@") {
				t.Fatalf("diff leaks plaintext: %+v", c)
			}
		}
	}
}

func TestCheckReason(t *testing.T) {
	if _, err := CheckReason("  "); detailCodes(err)["reason"] != "required" {
		t.Fatal("empty reason")
	}
	if _, err := CheckReason("ab"); detailCodes(err)["reason"] != "too_short" {
		t.Fatal("short reason")
	}
	if r, err := CheckReason("  Sözleşme  "); err != nil || r != "Sözleşme" {
		t.Fatalf("reason = %q, %v", r, err)
	}
}
