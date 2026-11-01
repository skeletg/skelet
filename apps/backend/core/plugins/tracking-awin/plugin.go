// Package trackingawin reports sales to Awin through its authenticated Conversion API, server to server, carrying the
// awc click id the storefront kept - the route Awin recommends over hand-built sread.php calls
package trackingawin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-awin"
	Kind = "awin"
)

var apiHost = "https://api.awin.com" // a var so tests can point it at a local server

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Awin (Conversion API)", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterDestination(sdk.Destination{
		Kind:             Kind,
		Label:            "Awin",
		Purposes:         []sdk.DestinationPurpose{sdk.PurposeAds},
		NecessarySetting: "general/privacy/awin_cookie_necessary",
		DefaultEvents:    []string{"purchase"},
		Fields: []sdk.DestinationField{
			{Key: "advertiser_id", Label: "Advertiser ID", Required: true},
			{Key: "api_key", Label: "API key", Help: "Awin > Account > API credentials", Required: true, Secret: true},
			{Key: "commission_group", Label: "Commission group", Help: "The group every sale commissions under - empty for DEFAULT"},
			{Key: "test", Label: "Test mode", Help: "true marks orders as tests, which Awin doesn't track"},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	e := ev.Ecommerce
	if ev.Name != "purchase" || e == nil || (ev.ClickIDs["awc"] == "" && e.Coupon == "") { // Awin needs a click or a voucher
		return nil
	}
	raw, err := json.Marshal(body{Orders: []order{toOrder(d, ev)}})
	if err != nil {
		return sdk.Permanent(err)
	}
	u := apiHost + "/s2s/advertiser/" + url.PathEscape(d.Config["advertiser_id"]) + "/orders"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return sdk.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", d.Secret)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := sdk.ResponseError(res); err != nil {
		return err
	}
	// 200, 202 and 206 all arrive here - a failed order inside them is a refusal of this sale
	var answer struct {
		FailedOrders []struct {
			Errors []struct{ Field, Message string } `json:"errors"`
		} `json:"failedOrders"`
	}
	if json.NewDecoder(res.Body).Decode(&answer) == nil && len(answer.FailedOrders) > 0 {
		msg := "refused"
		if errs := answer.FailedOrders[0].Errors; len(errs) > 0 {
			msg = errs[0].Field + ": " + errs[0].Message
		}
		return sdk.Permanent(fmt.Errorf("awin refused the order: %s", msg))
	}
	return nil
}

type body struct {
	Orders []order `json:"orders"`
}

type order struct {
	OrderReference   string            `json:"orderReference"`
	Amount           float64           `json:"amount"`
	Channel          string            `json:"channel"`
	Currency         string            `json:"currency"`
	CommissionGroups []commissionGroup `json:"commissionGroups"`
	Awc              string            `json:"awc,omitempty"`
	Voucher          string            `json:"voucher,omitempty"`
	IsTest           bool              `json:"isTest,omitempty"`
	Basket           []basketItem      `json:"basket,omitempty"`
}

type commissionGroup struct {
	Code   string  `json:"code"`
	Amount float64 `json:"amount"`
}

type basketItem struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Price               float64 `json:"price"`
	Quantity            int     `json:"quantity"`
	CommissionGroupCode string  `json:"commissionGroupCode"`
	Category            string  `json:"category"`
	SKU                 string  `json:"sku"`
}

func toOrder(d sdk.DestinationConfig, ev sdk.ShopEvent) order {
	e := ev.Ecommerce
	group := strings.ToUpper(strings.TrimSpace(d.Config["commission_group"]))
	if group == "" {
		group = "DEFAULT"
	}
	o := order{
		OrderReference: clip(url.QueryEscape(e.TransactionID), 50),
		Amount:         e.Value, // the subtotal after discounts, as Awin defines amount
		Channel:        "aw", Currency: e.Currency,
		CommissionGroups: []commissionGroup{{Code: group, Amount: e.Value}},
		Awc:              ev.ClickIDs["awc"], Voucher: url.QueryEscape(e.Coupon), IsTest: d.Config["test"] == "true",
	}
	for _, it := range e.Items {
		name, category := noPipes(it.ItemName), noPipes(it.ItemCategory)
		if name == "" {
			name = noPipes(it.ItemID)
		}
		if category == "" {
			category = "Uncategorised" // every basket field is required
		}
		o.Basket = append(o.Basket, basketItem{
			ID: clip(noPipes(it.ItemID), 75), Name: clip(name, 255), Price: it.Price, Quantity: max(1, int(math.Round(it.Quantity))),
			CommissionGroupCode: group, Category: clip(category, 255), SKU: clip(noPipes(it.ItemID), 255),
		})
	}
	return o
}

func noPipes(s string) string { return strings.ReplaceAll(s, "|", "-") }

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
