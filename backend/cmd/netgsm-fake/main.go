// Command netgsm-fake serves the in-process Netgsm fake (REST v2 send, otp,
// report, msgheader) for local end-to-end checks of SMS and OTP without
// network access or a real Netgsm account. Development tool only: point
// NETGSM_SANDBOX_BASE_URL at it and connect a Netgsm account in sandbox mode
// with the same usercode / password / msgheader. Received messages (OTP
// codes included) are listed on its own page, "/", never in the app logs.
//
//	go run ./cmd/netgsm-fake -addr 127.0.0.1:18091 -usercode 8500000000 -password pass -header RENTOPIA
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm/netgsmfake"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18091", "listen address")
	usercode := flag.String("usercode", "8500000000", "Netgsm usercode")
	password := flag.String("password", "sandbox-password", "Netgsm password")
	header := flag.String("header", "RENTOPIA", "defined message header")
	deliver := flag.Bool("deliver", true, "report messages delivered at once")
	flag.Parse()

	fake := netgsmfake.NewUnstarted()
	fake.AddAccount(*usercode, *password, *header)
	fake.AutoDeliver = *deliver
	srv := &http.Server{Addr: *addr, Handler: fake.Handler(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("netgsm fake listening on http://%s (inbox at /)", *addr)
	log.Fatal(srv.ListenAndServe())
}
