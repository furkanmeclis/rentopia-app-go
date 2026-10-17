package whatsapp

import (
	"net/url"
	"testing"
)

func TestParseWuzapiWebhook(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, ct, body string
		typ, receipt   string
		ids            int
	}{
		{"delivered", "application/json", `{"type":"ReadReceipt","state":"Delivered","event":{"MessageIDs":["A","B"],"Timestamp":"2026-10-16T10:00:00Z"}}`, EventReceipt, ReceiptDelivered, 2},
		{"read", "application/json; charset=utf-8", `{"type":"ReadReceipt","state":"Read","event":{"MessageIDs":["A"]}}`, EventReceipt, ReceiptRead, 1},
		{"type only", "application/json", `{"type":"ReadReceipt","event":{"messageIds":["A"],"Type":"read"}}`, EventReceipt, ReceiptRead, 1},
		{"other receipt", "application/json", `{"type":"ReadReceipt","state":"Retry","event":{"MessageIDs":["A"]}}`, EventOther, "", 0},
		{"connected", "application/json", `{"type":"Connected","event":{}}`, EventConnected, "", 0},
		{"paired", "application/json", `{"type":"PairSuccess","event":{"ID":"905321234567@s.whatsapp.net"}}`, EventPaired, "", 0},
		{"logged out", "application/json", `{"type":"LoggedOut","event":{"Reason":401}}`, EventLoggedOut, "", 0},
		{"message", "application/json", `{"type":"Message","event":{}}`, EventOther, "", 0},
		{"form", "application/x-www-form-urlencoded", "jsonData=" + url.QueryEscape(`{"type":"ReadReceipt","state":"Delivered","event":{"MessageIDs":["A"]}}`), EventReceipt, ReceiptDelivered, 1},
	}
	for _, c := range cases {
		evs, err := ParseWuzapiWebhook(c.ct, []byte(c.body))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(evs) != 1 || evs[0].Type != c.typ || evs[0].Receipt != c.receipt || len(evs[0].MessageIDs) != c.ids {
			t.Fatalf("%s: %+v", c.name, evs)
		}
	}
	if _, err := ParseWuzapiWebhook("application/json", []byte("{")); err == nil {
		t.Fatal("broken body parsed")
	}
}

func TestStatusPhone(t *testing.T) {
	t.Parallel()
	for jid, want := range map[string]string{
		"905321234567:12@s.whatsapp.net": "+905321234567",
		"905321234567@s.whatsapp.net":    "+905321234567",
		"":                               "",
		"abc@lid":                        "",
	} {
		if got := (Status{JID: jid}).Phone(); got != want {
			t.Fatalf("%q: %q", jid, got)
		}
	}
	if Digits(" +905321234567") != "905321234567" {
		t.Fatal("digits")
	}
}
