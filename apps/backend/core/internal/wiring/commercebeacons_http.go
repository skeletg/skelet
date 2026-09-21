package wiring

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

// Signup forms and "notify me when it's back" sit under /t so they ride the storefront's same-origin proxy, session cookie and all
// What they collect is commerce though - a lead, a consent record, a restock subscription

// The handlers behind signalsapi.CommerceBeaconPaths - the beacon tier forwards that list here, and
// a test holds this table to it
var commerceBeacons = []struct {
	path     string
	limitKey string // its own rate-limit bucket, like every /t endpoint
	handle   func(commerceBeaconDeps, http.ResponseWriter, *http.Request, uuid.UUID)
}{
	{"/t/back-in-stock", "bis", commerceBeaconDeps.backInStock},
	{"/t/forms", "forms", commerceBeaconDeps.activeForms},
	{"/t/forms/submit", "formsubmit", commerceBeaconDeps.submitForm},
}

type commerceBeaconDeps struct {
	Channels    *channel.Service
	BackInStock *backinstock.Service
	Forms       *form.Service
	Config      *config.Service
	Limiter     *ratelimit.RedisLimiter
	JWTSecret   string
}

// Public and unauthenticated like the rest of /t - per-IP, per-endpoint rate limited, channel from the header
func registerCommerceBeacons(mux *http.ServeMux, d commerceBeaconDeps) {
	guard := func(limitKey string, h func(commerceBeaconDeps, http.ResponseWriter, *http.Request, uuid.UUID)) http.Handler {
		return channel.Middleware(d.Channels)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ch, ok := channel.FromContext(r.Context())
			if !ok || ch == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if d.Limiter != nil && !d.Limiter.Allow(r.Context(), "ingest:"+limitKey+":"+auth.ClientIP(r)) { // an unconfigured limiter admits rather than refusing every shopper
				if r.Method == http.MethodGet {
					http.Error(w, "too many requests", http.StatusTooManyRequests)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
				return
			}
			h(d, w, r, ch.ID)
		}))
	}
	for _, b := range commerceBeacons {
		mux.Handle(b.path, guard(b.limitKey, b.handle))
	}
}

// "Notify me when it's back": public submit from the PDP, customer from the session cookie (else guest by email), best-effort 204
func (d commerceBeaconDeps) backInStock(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var b struct {
		ProductID string `json:"productId"`
		SKU       string `json:"sku"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		Image     string `json:"image"`
		URL       string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&b); err == nil {
		if pid, perr := uuid.Parse(b.ProductID); perr == nil && strings.TrimSpace(b.Email) != "" {
			req := backinstock.Request{
				ChannelID: channelID, ProductID: pid, SKU: b.SKU, Email: b.Email,
				AnonID: auth.SanitizeAnonID(r.Header.Get(auth.AnonHeader)), Name: b.Name, Image: b.Image, URL: b.URL,
			}
			if cid := auth.VerifiedStorefrontCustomer(r, d.JWTSecret); cid != uuid.Nil {
				req.CustomerID = &cid
			}
			_ = d.BackInStock.Record(r.Context(), req)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Active signup forms for the storefront widget - public-safe projection only (no target segment / consent internals)
func (d commerceBeaconDeps) activeForms(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if on, _ := d.Config.GetBool(r.Context(), "outreach/forms/enabled", config.DefaultScope); !on {
		_ = json.NewEncoder(w).Encode([]any{}) // master switch off
		return
	}
	forms, _ := d.Forms.Active(r.Context(), channelID)
	type pubForm struct {
		ID             string             `json:"id"`
		Slug           string             `json:"slug"`
		Type           string             `json:"type"`
		Heading        string             `json:"heading"`
		Body           string             `json:"body"`
		ButtonLabel    string             `json:"buttonLabel"`
		SuccessMessage string             `json:"successMessage"`
		CollectName    bool               `json:"collectName"`
		Display        form.DisplayConfig `json:"display"`
	}
	out := make([]pubForm, 0, len(forms))
	for _, f := range forms {
		out = append(out, pubForm{
			ID: f.ID.String(), Slug: f.Slug, Type: f.Type, Heading: f.Heading, Body: f.Body,
			ButtonLabel: f.ButtonLabel, SuccessMessage: f.SuccessMessage, CollectName: f.CollectName, Display: f.Display,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

// Records the lead, ensures a marketing contact, opts them in, adds to the segment, fires the welcome flow - returns the success message
func (d commerceBeaconDeps) submitForm(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var b struct {
		Form  string `json:"form"` // slug or id
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&b); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
		return
	}
	msg, err := d.Forms.Submit(r.Context(), channelID, b.Form, b.Email, b.Name)
	if err != nil {
		// Only the two shopper-facing sentinels get echoed - any other text would leak internals
		switch {
		case errors.Is(err, form.ErrEmailRequired), errors.Is(err, form.ErrFormNotFound):
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		default:
			slog.Error("signup form submit", "channel", channelID, "form", b.Form, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "could not submit right now"})
		}
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
