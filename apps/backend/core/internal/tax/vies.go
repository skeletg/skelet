package tax

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// VIES VAT-id validation against the EU Commission's REST API - advisory, so a network or service failure yields "unavailable" and the id is stored anyway for a later re-check, never a save-blocking error
// Reverse-charge decisions stay with the operator; this only records what VIES said

const (
	VatStatusUnverified  = "unverified"
	VatStatusValid       = "valid"
	VatStatusInvalid     = "invalid"
	VatStatusUnavailable = "unavailable"
)

// A var so tests can point the client at a stub server
var viesBaseURL = "https://ec.europa.eu/taxation_customs/vies/rest-api"

// The member-state codes VIES answers for - Greece uses EL rather than GR in VAT ids, and XI is Northern Ireland post-Brexit
var euVatCountries = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DE": true,
	"DK": true, "EE": true, "EL": true, "ES": true, "FI": true, "FR": true,
	"HR": true, "HU": true, "IE": true, "IT": true, "LT": true, "LU": true,
	"LV": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true,
	"SE": true, "SI": true, "SK": true, "XI": true,
}

var vatBodyRe = regexp.MustCompile(`^[0-9A-Za-z+*.]{2,12}$`)

// Uppercases, strips spaces, dots and dashes, then splits into member-state prefix plus number - errors before any network call on anything that can't be a VIES-checkable id
func NormalizeVATID(raw string) (country, number string, err error) {
	cleaned := strings.ToUpper(strings.NewReplacer(" ", "", ".", "", "-", "").Replace(strings.TrimSpace(raw)))
	if len(cleaned) < 4 {
		return "", "", fmt.Errorf("tax: VAT id too short")
	}
	country, number = cleaned[:2], cleaned[2:]
	if !euVatCountries[country] {
		return "", "", fmt.Errorf("tax: %q is not an EU VAT country prefix", country)
	}
	if !vatBodyRe.MatchString(number) {
		return "", "", fmt.Errorf("tax: VAT number %q has invalid format", number)
	}
	return country, number, nil
}

type VatCheckResult struct {
	VatID  string
	Status string
	// Name and Address echo VIES's registered trader details when valid, often "---" for member states that suppress them
	Name    string
	Address string
}

// Normalizes and checks a VAT id against VIES - a format error returns a best-effort result plus the error, while a network or VIES failure returns Status "unavailable" with a nil error
func ValidateVATID(ctx context.Context, raw string) (VatCheckResult, error) {
	country, number, err := NormalizeVATID(raw)
	if err != nil {
		return VatCheckResult{VatID: strings.ToUpper(strings.TrimSpace(raw)), Status: VatStatusUnverified}, err
	}
	res := VatCheckResult{VatID: country + number, Status: VatStatusUnavailable}

	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/ms/%s/vat/%s", viesBaseURL, country, number), nil)
	if err != nil {
		return res, nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, nil
	}
	var payload struct {
		IsValid bool   `json:"isValid"`
		Name    string `json:"name"`
		Address string `json:"address"`
		// VIES reports member-state outages through userError codes like MS_UNAVAILABLE and MS_MAX_CONCURRENT_REQ
		UserError string `json:"userError"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return res, nil
	}
	if ue := strings.ToUpper(payload.UserError); ue != "" && ue != "VALID" && ue != "INVALID" {
		return res, nil
	}
	if payload.IsValid {
		res.Status = VatStatusValid
		res.Name = payload.Name
		res.Address = payload.Address
	} else {
		res.Status = VatStatusInvalid
	}
	return res, nil
}
