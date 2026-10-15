// Command iyzico-fake serves the in-process iyzico fake (Checkout Form
// initialize / retrieve, refund v2, BIN check and a hosted test page) for
// local end-to-end checks of the payment flow without network access or
// real sandbox keys. Development tool only: point IYZICO_SANDBOX_BASE_URL at
// it and connect an iyzico account with the same keys.
//
//	go run ./cmd/iyzico-fake -addr 127.0.0.1:18090 -api-key k -secret-key s \
//	    -webhook http://127.0.0.1:8080/v1/public/payments/iyzico/webhook
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18090", "listen address")
	apiKey := flag.String("api-key", "sandbox-api-key", "merchant api key")
	secretKey := flag.String("secret-key", "sandbox-secret-key", "merchant secret key")
	webhook := flag.String("webhook", "", "webhook URL notified when the hosted page completes")
	flag.Parse()

	fake := iyzicofake.NewUnstarted()
	fake.AddAccount(*apiKey, *secretKey)
	fake.WebhookURL = *webhook
	srv := &http.Server{Addr: *addr, Handler: fake.Handler(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("iyzico fake listening on http://%s", *addr)
	log.Fatal(srv.ListenAndServe())
}
