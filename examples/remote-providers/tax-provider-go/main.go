// A complete, runnable Skelet tax_provider. Standard library only - copy it, replace
// calculate(), and you have a provider.
//
//	SKELET_PROVIDER_SECRET=rpsec_... go run .
//	bin/provider-check --url http://localhost:9095/tax --secret rpsec_... --kind tax_provider
//
// The contract it implements is docs/remote-providers.md. The three things that actually
// trip people up are all handled below and flagged with WHY comments: signing over the
// HEADER timestamp (not the one in the body), comparing the digest in constant time, and
// answering with a number rather than an error when the answer is legitimately zero.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Anything older than this is refused. Core does not cap the age itself, so without this a
// single captured request stays replayable forever.
const maxSkew = 5 * time.Minute

// The envelope core POSTs. `config` is whatever JSON the merchant saved on the provider row
// in the admin - typically credentials and tuning for your service.
type envelope struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	// The wire-contract version core built this request under (also the X-Provider-Contract-Version
	// header) - worth logging so a payload change is traceable to a core upgrade, and pre-1.0 a
	// minor bump means something you relied on may have changed shape
	ContractVersion string          `json:"contractVersion"`
	Timestamp       string          `json:"timestamp"`
	Config          json.RawMessage `json:"config"`
	Payload         taxRequest      `json:"payload"`
}

// Generated from core's own struct: internal/remoteprovider/contract/tax_provider.schema.json
type taxRequest struct {
	ChannelID     string   `json:"channelId"`
	Currency      string   `json:"currency"`
	TaxInclusive  bool     `json:"taxInclusive"`
	SubtotalCents int64    `json:"subtotalCents"`
	DiscountCents int64    `json:"discountCents"`
	CustomerGroup string   `json:"customerGroup"`
	TaxExempt     bool     `json:"taxExempt"`
	Address       address  `json:"address"`
	Lines         []txLine `json:"lines"`
}

type address struct {
	Street1     string `json:"street1"`
	Street2     string `json:"street2"`
	City        string `json:"city"`
	Region      string `json:"region"`
	PostalCode  string `json:"postalCode"`
	CountryCode string `json:"countryCode"`
}

type txLine struct {
	SKU            string  `json:"sku"`
	Quantity       float64 `json:"quantity"`
	UnitPriceCents int64   `json:"unitPriceCents"`
}

// taxCents is a POINTER in core. Sending null (or omitting it) is read as "this provider failed", not as zero tax, and
// core then applies the configured failure policy. Always send a number - including 0 for a genuinely untaxed order
type taxResponse struct {
	TaxCents *int64 `json:"taxCents"`
}

func main() {
	secret := os.Getenv("SKELET_PROVIDER_SECRET")
	if secret == "" {
		log.Fatal("set SKELET_PROVIDER_SECRET to the rpsec_ value shown when the provider was registered")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":9095"
	}
	http.HandleFunc("/tax", handler(secret))
	log.Printf("tax provider listening on %s/tax", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		// Read the body before parsing: the signature covers the exact bytes, so re-encoding
		// anything before verifying would compare a different message
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		if err := verify(secret, r.Header, body); err != nil {
			// 401 keeps it out of your success metrics and tells core to apply its failure
			// policy rather than treat the reply as an answer
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			http.Error(w, "bad envelope", http.StatusBadRequest)
			return
		}

		cents := calculate(env.Payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(taxResponse{TaxCents: &cents})
	}
}

func verify(secret string, h http.Header, body []byte) error {
	got := strings.TrimPrefix(h.Get("X-Provider-Signature-256"), "sha256=")
	if got == "" {
		return errUnsigned
	}
	// WHY the header and not env.Timestamp: core computes the RFC3339 field in the body and the unix-seconds header at different moments, and
	// signs with the HEADER one. Signing over the body's timestamp yields a digest that never matches
	ts := h.Get("X-Provider-Timestamp")
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errBadTimestamp
	}
	if age := time.Since(time.Unix(secs, 0)); age > maxSkew || age < -maxSkew {
		return errStale
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	// Constant time - a plain == leaks the digest a byte at a time under timing analysis
	if !hmac.Equal([]byte(got), []byte(want)) {
		return errBadSignature
	}
	return nil
}

type provErr string

func (e provErr) Error() string { return string(e) }

const (
	errUnsigned     = provErr("missing signature")
	errBadTimestamp = provErr("bad timestamp")
	errStale        = provErr("timestamp outside the accepted window")
	errBadSignature = provErr("signature mismatch")
)

// Replace this with a call to your tax engine. Flat 8.75% on the discounted subtotal, which
// is enough to prove the wiring end to end.
func calculate(req taxRequest) int64 {
	if req.TaxExempt {
		return 0
	}
	taxable := req.SubtotalCents - req.DiscountCents
	if taxable < 0 {
		taxable = 0
	}
	return taxable * 875 / 10000
}
