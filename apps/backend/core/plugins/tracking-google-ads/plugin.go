// Package trackinggoogleads sends purchases to Google Ads as conversions, server to server, through Google's Data
// Manager API. The click ids the storefront kept with ads consent (gclid, gbraid, wbraid) attribute each order to its
// ad click, and the shopper's hashed email and phone recover the ones whose click id is gone - Google's enhanced
// conversions. No Google script runs in the browser for it
package trackinggoogleads

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug   = "tracking-google-ads"
	Kind   = "google_ads"
	scope  = "https://www.googleapis.com/auth/datamanager"
	maxAge = 90 * 24 * time.Hour // a click id is good for 90 days
)

var (
	apiHost     = "https://datamanager.googleapis.com"  // vars so tests can point them at a local server
	googleToken = "https://oauth2.googleapis.com/token" // where a refresh token is exchanged; a service account names its own
)

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Google Ads conversions", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:          Kind,
		Label:         "Google Ads conversions",
		Purposes:      []sdk.DestinationPurpose{sdk.PurposeAds},
		DefaultEvents: []string{"purchase"},
		Fields: []sdk.DestinationField{
			{Key: "customer_id", Label: "Google Ads customer ID", Help: "The account the conversions belong to, digits only (123-456-7890 becomes 1234567890)", Required: true},
			{Key: "conversion_action_id", Label: "Conversion action ID", Help: "A conversion action whose source is Website (Import from clicks)", Required: true},
			{Key: "login_customer_id", Label: "Manager account ID", Help: "Only when the credentials reach the account through a manager account - empty uses the account itself"},
			{Key: "credentials", Label: "Google credentials JSON", Help: "A service account key, or an authorized-user file with a refresh token. Its account needs access to the Google Ads account, and the Data Manager API enabled in its Cloud project", Required: true, Secret: true},
			{Key: "validate_only", Label: "Validate only", Help: "true has Google check each conversion and record nothing - for setting up"},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	if ev.Name != "purchase" || ev.Ecommerce == nil {
		return nil
	}
	if time.Since(ev.OccurredAt) > maxAge {
		return sdk.Permanent(fmt.Errorf("%s is older than a Google click id's 90 days", ev.ID))
	}
	customer := digits(d.Config["customer_id"])
	action := strings.TrimSpace(d.Config["conversion_action_id"])
	if customer == "" || action == "" {
		return sdk.Permanent(errors.New("google ads: customer_id and conversion_action_id are required"))
	}
	e, ok := toEvent(ev)
	if !ok {
		return nil // no click id and no email or phone - Google could match it to nobody
	}
	login := digits(d.Config["login_customer_id"])
	if login == "" {
		login = customer
	}
	body := request{
		Destinations: []destination{{
			OperatingAccount: account{Type: "GOOGLE_ADS", ID: customer}, LoginAccount: account{Type: "GOOGLE_ADS", ID: login}, ProductDestinationID: action,
		}},
		Encoding:     "HEX",
		Events:       []event{e},
		ValidateOnly: d.Config["validate_only"] == "true",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return sdk.Permanent(err)
	}
	token, err := accessToken(ctx, client, d.ID.String(), d.Secret)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiHost+"/v1/events:ingest", bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		forget(d.ID.String()) // a revoked or expired token - the retry fetches a fresh one
	}
	return sdk.ResponseError(res)
}

// The shape of the Data Manager API's events:ingest request:
// https://developers.google.com/data-manager/api/reference/rest/v1/events/ingest
type request struct {
	Destinations []destination `json:"destinations"`
	Encoding     string        `json:"encoding"`
	Events       []event       `json:"events"`
	ValidateOnly bool          `json:"validateOnly,omitempty"`
}

type destination struct {
	OperatingAccount     account `json:"operatingAccount"`
	LoginAccount         account `json:"loginAccount"`
	ProductDestinationID string  `json:"productDestinationId"`
}

type account struct {
	Type string `json:"accountType"`
	ID   string `json:"accountId"`
}

type event struct {
	TransactionID   string         `json:"transactionId"`
	EventTimestamp  string         `json:"eventTimestamp"`
	EventSource     string         `json:"eventSource"`
	ConversionValue float64        `json:"conversionValue"`
	Currency        string         `json:"currency"`
	AdIdentifiers   *adIdentifiers `json:"adIdentifiers,omitempty"`
	UserData        *userData      `json:"userData,omitempty"`
	Consent         consent        `json:"consent"`
}

type adIdentifiers struct {
	Gclid  string `json:"gclid,omitempty"`
	Gbraid string `json:"gbraid,omitempty"`
	Wbraid string `json:"wbraid,omitempty"`
}

type userData struct {
	UserIdentifiers []identifier `json:"userIdentifiers"`
}

type identifier struct {
	EmailAddress string `json:"emailAddress,omitempty"`
	PhoneNumber  string `json:"phoneNumber,omitempty"`
}

type consent struct {
	AdUserData        string `json:"adUserData"`
	AdPersonalization string `json:"adPersonalization"`
}

func toEvent(ev sdk.ShopEvent) (event, bool) {
	e := ev.Ecommerce
	out := event{
		TransactionID:  e.TransactionID,
		EventTimestamp: ev.OccurredAt.UTC().Format(time.RFC3339),
		EventSource:    "WEB",
		// What was paid, tax and shipping in - the same order value the other ad platforms receive
		ConversionValue: e.Value + e.Tax + e.Shipping,
		Currency:        e.Currency,
	}
	if out.TransactionID == "" {
		out.TransactionID = ev.ID
	}
	ids := adIdentifiers{Gclid: ev.ClickIDs["gclid"], Gbraid: ev.ClickIDs["gbraid"], Wbraid: ev.ClickIDs["wbraid"]}
	if ids != (adIdentifiers{}) {
		out.AdIdentifiers = &ids
	}
	var who []identifier
	if em := normalizeEmail(ev.User.Email); em != "" {
		who = append(who, identifier{EmailAddress: hash(em)})
	}
	if ph := phoneDigits(ev.User.Phone); ph != "" {
		who = append(who, identifier{PhoneNumber: hash("+" + ph)})
	}
	if len(who) > 0 {
		out.UserData = &userData{UserIdentifiers: who}
	}
	if ev.Consent != nil {
		out.Consent = consent{AdUserData: status(ev.Consent.Ads), AdPersonalization: status(ev.Consent.Personalization)}
	} else {
		out.Consent = consent{AdUserData: "CONSENT_DENIED", AdPersonalization: "CONSENT_DENIED"}
	}
	return out, out.AdIdentifiers != nil || out.UserData != nil
}

func status(granted bool) string {
	if granted {
		return "CONSENT_GRANTED"
	}
	return "CONSENT_DENIED"
}

// Google's normalization before hashing: lowercase, no whitespace, and for Gmail no dots or +suffix in the local part
// https://developers.google.com/data-manager/api/devguides/concepts/formatting
func normalizeEmail(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), ""))
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" {
		return ""
	}
	if domain == "gmail.com" || domain == "googlemail.com" {
		local, _, _ = strings.Cut(local, "+")
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Digits without a trunk or international prefix's leading zeros; hashed as E.164 with its +. A number typed without
// its country code won't match, as on Meta
func phoneDigits(s string) string {
	return strings.TrimLeft(digits(s), "0")
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Access tokens live an hour, so one is reused for every send until shortly before it expires. Keyed by the
// destination and its credentials, so replacing the credentials takes effect at once
var tokens struct {
	sync.Mutex
	byKey map[string]cachedToken
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

func forget(destination string) {
	tokens.Lock()
	defer tokens.Unlock()
	for k := range tokens.byKey {
		if strings.HasPrefix(k, destination+":") {
			delete(tokens.byKey, k)
		}
	}
}

type credentials struct {
	Type         string `json:"type"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	TokenURI     string `json:"token_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

func accessToken(ctx context.Context, client *http.Client, destination, secret string) (string, error) {
	key := destination + ":" + hash(secret)
	tokens.Lock()
	if t, ok := tokens.byKey[key]; ok && time.Until(t.expiresAt) > time.Minute {
		tokens.Unlock()
		return t.value, nil
	}
	tokens.Unlock()

	var c credentials
	if err := json.Unmarshal([]byte(secret), &c); err != nil {
		return "", sdk.Permanent(errors.New("google ads: the credentials are not a Google credentials JSON file"))
	}
	var form url.Values
	endpoint := googleToken
	switch c.Type {
	case "service_account":
		assertion, err := signedJWT(c, time.Now())
		if err != nil {
			return "", sdk.Permanent(err)
		}
		if c.TokenURI != "" {
			endpoint = c.TokenURI
		}
		form = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	case "authorized_user":
		if c.ClientID == "" || c.ClientSecret == "" || c.RefreshToken == "" {
			return "", sdk.Permanent(errors.New("google ads: the authorized-user credentials need client_id, client_secret and refresh_token"))
		}
		form = url.Values{"grant_type": {"refresh_token"}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "refresh_token": {c.RefreshToken}}
	default:
		return "", sdk.Permanent(fmt.Errorf("google ads: credentials of type %q are not supported - use a service account key or an authorized-user file", c.Type))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", sdk.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if err := sdk.ResponseError(res); err != nil {
		return "", fmt.Errorf("google ads token: %w", err) // 400/401 here is bad credentials, which ResponseError already makes permanent
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil || t.AccessToken == "" {
		return "", errors.New("google ads token: no access token in the answer")
	}
	tokens.Lock()
	if tokens.byKey == nil {
		tokens.byKey = map[string]cachedToken{}
	}
	tokens.byKey[key] = cachedToken{value: t.AccessToken, expiresAt: time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)}
	tokens.Unlock()
	return t.AccessToken, nil
}

// A service account's OAuth assertion: an RS256 JWT for the Data Manager scope, signed with its private key
func signedJWT(c credentials, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(c.PrivateKey))
	if block == nil || c.ClientEmail == "" {
		return "", errors.New("google ads: the service account key has no client_email or PEM private_key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("google ads: the service account private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("google ads: the service account private key is not RSA")
	}
	aud := c.TokenURI
	if aud == "" {
		aud = googleToken
	}
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{"iss": c.ClientEmail, "scope": scope, "aud": aud, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + enc.EncodeToString(sig), nil
}
