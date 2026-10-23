// Package einvoice is the provider layer of e-invoices (e-Fatura) and
// e-archive invoices (e-Arşiv) (TEC-722, docs/design/einvoice.md).
//
// Documents are produced and validated by the invoices module with go-ubltr;
// a provider (a GİB-authorized private integrator, "özel entegratör") only
// transports them: Send a signed-ready UBL-TR document, ask its Status by
// our ETTN (the invoice UUID, generated before the first send), Cancel an
// e-archive invoice, Download the document the integrator keeps, and look a
// taxpayer up ("mükellef sorgusu": is the receiver registered for e-Fatura).
//
// The integrator is NOT chosen yet (TEC-685). The official slot (Official,
// provider "einvoice") can be connected with the organization's credentials
// but reports ErrDependencyPending ("waiting on dependency") until it is;
// invoices for it stay visible in that state and are never sent. The
// development / test adapter (package einvoicefake, provider
// "einvoice_fake") is registered outside production only.
package einvoice

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// Kind is the integration kind of e-invoice accounts.
const Kind = "einvoice"

// ProviderOfficial is the provider code of the official (integrator) slot.
const ProviderOfficial = "einvoice"

// Document formats Download returns.
const (
	FormatXML = "xml"
	FormatPDF = "pdf"
)

// States a status query answers.
const (
	// StateProcessing: accepted by the integrator, GİB has not answered yet.
	StateProcessing = "processing"
	// StateSucceeded: delivered (e-Fatura: GİB accepted it; e-Arşiv: the
	// integrator issued and reported it).
	StateSucceeded = "succeeded"
	// StateFailed: refused (GİB or the integrator); nothing was issued.
	StateFailed = "failed"
	// StateCancelled: an e-archive invoice that was cancelled.
	StateCancelled = "cancelled"
)

// Document is one UBL-TR invoice to transport.
type Document struct {
	// UUID is the ETTN: generated before the first send, so a repeated send
	// of the same document can be recognised by the integrator and by our
	// status query (the idempotency key of the whole flow).
	UUID    string
	Number  string
	Profile string
	Type    string
	// Receiver is the receiver's VKN / TCKN; ReceiverAlias its e-Fatura
	// mailbox ("urn:mail:defaultpk@…") from the taxpayer lookup, empty for
	// e-Arşiv.
	Receiver      string
	ReceiverAlias string
	XML           []byte
}

// SendResult is the integrator's answer to a send.
type SendResult struct {
	// Reference is the integrator's own id of the document.
	Reference string
	// State is StateProcessing (accepted, result later) or StateSucceeded.
	State string
}

// StatusResult is the answer of a status query.
type StatusResult struct {
	// Found = the integrator has a document with our UUID.
	Found     bool
	State     string
	Reference string
	Message   string
}

// Taxpayer is the answer of a taxpayer lookup.
type Taxpayer struct {
	Identifier string
	// EInvoice = registered for e-Fatura (send TEMEL / TICARI to Alias);
	// otherwise the receiver gets an e-Arşiv invoice.
	EInvoice bool
	Title    string
	Alias    string
}

// Provider is one e-invoice integrator adapter.
type Provider interface {
	integrations.Adapter
	// Ready reports whether the adapter can send at all: nil, or
	// ErrDependencyPending (the integrator is not chosen yet).
	Ready() error
	// Send transports one document. ErrUnavailable: the request did not
	// leave (connection refused, DNS): nothing was sent. ErrRejected: the
	// integrator refused it (definitive). Any other error: the outcome is
	// unknown (a timeout after sending) — the caller asks Status first.
	Send(ctx context.Context, account integrations.Account, doc Document) (SendResult, error)
	// Status asks what happened to the document with our UUID (and the
	// integrator's reference when known). An error means "unknown".
	Status(ctx context.Context, account integrations.Account, uuid, reference string) (StatusResult, error)
	// Cancel cancels an e-archive invoice (e-Fatura cannot be cancelled by
	// the integrator: a return invoice is issued instead).
	Cancel(ctx context.Context, account integrations.Account, uuid, reference, reason string) error
	// Download returns the document the integrator keeps (FormatXML /
	// FormatPDF); integrations.ErrUnsupported when it has none.
	Download(ctx context.Context, account integrations.Account, uuid, reference, format string) ([]byte, error)
	// LookupTaxpayer answers whether a VKN / TCKN is an e-Fatura taxpayer.
	LookupTaxpayer(ctx context.Context, account integrations.Account, identifier string) (Taxpayer, error)
}

var (
	// ErrDependencyPending: no integrator is chosen yet (TEC-685); nothing
	// can be sent.
	ErrDependencyPending = errors.New("einvoice: waiting on dependency: the integrator is not chosen yet")
	// ErrUnavailable: the request did not reach the integrator; nothing was
	// sent, sending again is safe.
	ErrUnavailable = errors.New("einvoice: integrator unavailable (nothing was sent)")
	// ErrRejected: the integrator / GİB refused the document.
	ErrRejected = errors.New("einvoice: the document was rejected")
	// ErrTimeout: no answer after the request left; the outcome is unknown.
	ErrTimeout = errors.New("einvoice: no answer after sending (outcome unknown)")
	// ErrNotCancellable: the document cannot be cancelled (not e-Arşiv, or
	// past the cancellation window).
	ErrNotCancellable = errors.New("einvoice: the document cannot be cancelled")
)

// Official is the integrator slot. Until TEC-685 chooses the integrator it
// stores the organization's credentials but cannot send: Check and Ready
// answer ErrDependencyPending, every call ErrUnsupported (never made).
type Official struct{}

// Kind implements integrations.Adapter.
func (Official) Kind() string { return Kind }

// Provider implements integrations.Adapter.
func (Official) Provider() string { return ProviderOfficial }

// Fields implements integrations.Adapter: the integrator's API user and
// password (to be confirmed in TEC-685).
func (Official) Fields() []integrations.Field {
	return []integrations.Field{
		{Key: "username", LabelKey: "payments.integrations.fields.username", Secret: false, Required: true},
		{Key: "password", LabelKey: "payments.integrations.fields.password", Secret: true, Required: true},
	}
}

// Check implements integrations.Adapter.
func (Official) Check(context.Context, integrations.Account) error { return ErrDependencyPending }

// Ready implements Provider.
func (Official) Ready() error { return ErrDependencyPending }

// Send implements Provider (never called: Ready fails).
func (Official) Send(context.Context, integrations.Account, Document) (SendResult, error) {
	return SendResult{}, integrations.ErrUnsupported
}

// Status implements Provider (never called: Ready fails).
func (Official) Status(context.Context, integrations.Account, string, string) (StatusResult, error) {
	return StatusResult{}, integrations.ErrUnsupported
}

// Cancel implements Provider (never called: Ready fails).
func (Official) Cancel(context.Context, integrations.Account, string, string, string) error {
	return integrations.ErrUnsupported
}

// Download implements Provider (never called: Ready fails).
func (Official) Download(context.Context, integrations.Account, string, string, string) ([]byte, error) {
	return nil, integrations.ErrUnsupported
}

// LookupTaxpayer implements Provider (never called: Ready fails).
func (Official) LookupTaxpayer(context.Context, integrations.Account, string) (Taxpayer, error) {
	return Taxpayer{}, integrations.ErrUnsupported
}

// Guard wraps the official integrator adapter outside production (TEC-744,
// platform/envguard): with a non-nil refusal it never reaches the
// integrator. Every call answers the refusal, so sends are held (waiting
// on dependency) and nothing leaves. A nil refusal returns p unchanged.
func Guard(p Provider, refusal error) Provider {
	if refusal == nil || p == nil {
		return p
	}
	return guarded{inner: p, refusal: refusal}
}

type guarded struct {
	inner   Provider
	refusal error
}

func (g guarded) Kind() string                 { return g.inner.Kind() }
func (g guarded) Provider() string             { return g.inner.Provider() }
func (g guarded) Fields() []integrations.Field { return g.inner.Fields() }

func (g guarded) Check(context.Context, integrations.Account) error { return g.refusal }
func (g guarded) Ready() error                                      { return g.refusal }
func (g guarded) Send(context.Context, integrations.Account, Document) (SendResult, error) {
	return SendResult{}, g.refusal
}
func (g guarded) Status(context.Context, integrations.Account, string, string) (StatusResult, error) {
	return StatusResult{}, g.refusal
}
func (g guarded) Cancel(context.Context, integrations.Account, string, string, string) error {
	return g.refusal
}
func (g guarded) Download(context.Context, integrations.Account, string, string, string) ([]byte, error) {
	return nil, g.refusal
}
func (g guarded) LookupTaxpayer(context.Context, integrations.Account, string) (Taxpayer, error) {
	return Taxpayer{}, g.refusal
}
