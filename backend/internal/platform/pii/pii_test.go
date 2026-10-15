package pii

import "testing"

func TestChecksums(t *testing.T) {
	for _, v := range []string{"10000000146", "11111111110"} {
		if !ValidTCKN(v) {
			t.Errorf("ValidTCKN(%s) = false", v)
		}
	}
	for _, v := range []string{"10000000147", "01234567890", "1234567890", "1000000014a"} {
		if ValidTCKN(v) {
			t.Errorf("ValidTCKN(%s) = true", v)
		}
	}
	if !ValidVKN("4540536920") || ValidVKN("4540536921") || ValidVKN("454053692") {
		t.Error("ValidVKN")
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"0532 123 45 12":    "+905321234512",
		"532 123 45 12":     "+905321234512",
		"+90 (532) 1234512": "+905321234512",
		"905321234512":      "+905321234512",
		"0049 151 12345678": "+4915112345678",
		"":                  "",
		"abc":               "",
	}
	for in, want := range cases {
		if got := Phone(in); got != want {
			t.Errorf("Phone(%q) = %q, want %q", in, got, want)
		}
	}
	if !ValidPhone("+905321234512") || ValidPhone("+90532123451") || ValidPhone("+12") {
		t.Error("ValidPhone")
	}
	if Document(" u 1234-5678 ") != "U12345678" {
		t.Error("Document")
	}
	if Digits("100 000 001 46") != "10000000146" {
		t.Error("Digits")
	}
}

func TestMasking(t *testing.T) {
	cases := []struct{ got, want string }{
		{MaskIdentity(KindTCKN, "12345678945"), "123******45"},
		{MaskIdentity(KindVKN, "4540536920"), "454*****20"},
		{MaskIdentity(KindPassport, "U12345678"), "U******78"},
		{MaskIdentity(KindLicence, "B123456"), "****456"},
		{MaskIdentity(KindLicence, "12"), "**"},
		{MaskIdentity(KindTCKN, ""), ""},
		{MaskPhone("+905321234512"), "+90 5** *** ** 12"},
		{MaskPhone("+4915112345678"), "+49 1** ****** 78"},
		{MaskEmail("ayse.yilmaz@domain.com"), "a***@d***.com"},
		{MaskEmail("x@localhost"), "x***@l***"},
		{MaskEmail("broken"), "***"},
		{LastDigits("+905321234512", 4), "4512"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q, want %q", i, c.got, c.want)
		}
	}
}
