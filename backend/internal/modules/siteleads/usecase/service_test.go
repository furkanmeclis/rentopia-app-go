package usecase

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/model"
	"strings"
	"testing"
)

func TestValidationByKind(t *testing.T) {
	for _, kind := range model.Kinds {
		t.Run(kind, func(t *testing.T) {
			in := model.Input{Kind: kind, Email: " USER@EXAMPLE.COM ", ConsentKVKK: true}
			err := Validate(&in)
			if kind == "newsletter" && err != nil {
				t.Fatal(err)
			}
			if kind != "newsletter" && err == nil {
				t.Fatal("required fields missing")
			}
			in.Name = "Ada"
			in.Phone = "555"
			in.Company = "Company"
			in.City = "İzmir"
			in.Message = "Question"
			if err := Validate(&in); err != nil {
				t.Fatal(err)
			}
			if in.Email != "user@example.com" || in.Locale != "tr" {
				t.Fatal(in)
			}
		})
	}
	for _, in := range []model.Input{
		{Kind: "newsletter", Email: "bad", ConsentKVKK: true},
		{Kind: "newsletter", Email: "a@b.test"},
		{Kind: "unknown", Email: "a@b.test", ConsentKVKK: true},
		{Kind: "newsletter", Email: "a@b.test", ConsentKVKK: true, Locale: "de"},
		{Kind: "newsletter", Email: "a@b.test", ConsentKVKK: true, Name: strings.Repeat("ş", 161)},
	} {
		if Validate(&in) == nil {
			t.Fatalf("accepted invalid input %+v", in)
		}
	}
}
