package shoptools

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

// The order tools are the storefront assistant's alone - a shopper's agent over MCP is anonymous and has no orders to read
const (
	orderPagePath    = "/account/orders/"
	maxOrdersListed  = 10
	maxOrderEvents   = 5
	maxReasonLen     = 500
	notSignedIn      = "The shopper isn't signed in, so their orders aren't open to you. Ask them to sign in to their account on this store, then ask again."
	resolutionRefund = "refund"
)

var (
	myOrdersDoc = document("myOrders", `query AgentMyOrders {
  myOrders { id number status paymentStatus total currencyCode createdAt carrier trackingNumber lines { quantity } }
}`)
	myOrderDoc = document("myOrder", `query AgentMyOrder($id: ID!) {
  myOrder(id: $id) {
    id number status paymentStatus paymentInstructions total subtotal shippingAmount discountAmount taxAmount amountDue currencyCode createdAt
    carrier trackingNumber trackingUrl shippingAddress { city countryCode }
    lines { id name sku quantity totalPrice isGift }
    events { type message createdAt }
    returns { rmaNumber status resolution createdAt lines { orderLineId quantity } }
    returnableLines { orderLineId quantity }
  }
}`)
	requestReturnDoc = document("requestReturn", `mutation AgentRequestReturn($input: RequestReturnInput!) {
  requestReturn(input: $input) { rmaNumber status resolution }
}`)
)

var orderStatusWords = map[string]string{
	"PENDING": "placed, waiting to be confirmed", "CONFIRMED": "confirmed", "ON_HOLD": "on hold", "PROCESSING": "being prepared",
	"PARTIALLY_FULFILLED": "partly shipped", "FULFILLED": "shipped", "CANCELLED": "cancelled",
}

var paymentStatusWords = map[string]string{
	"UNPAID": "not paid yet", "AUTHORIZED": "payment authorised", "PAID": "paid", "REFUNDED": "refunded", "VOIDED": "payment voided",
}

func inWords(m map[string]string, v string) string {
	if w, ok := m[v]; ok {
		return w
	}
	return strings.ToLower(v)
}

type orderSummary struct {
	ID             string    `json:"id"`
	Number         string    `json:"number"`
	Status         string    `json:"status"`
	PaymentStatus  string    `json:"paymentStatus"`
	Total          int64     `json:"total"`
	CurrencyCode   string    `json:"currencyCode"`
	CreatedAt      time.Time `json:"createdAt"`
	Carrier        string    `json:"carrier"`
	TrackingNumber string    `json:"trackingNumber"`
	Lines          []struct {
		Quantity float64 `json:"quantity"`
	} `json:"lines"`
}

type orderLine struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	SKU        string  `json:"sku"`
	Quantity   float64 `json:"quantity"`
	TotalPrice int64   `json:"totalPrice"`
	IsGift     bool    `json:"isGift"`
}

type lineQuantity struct {
	OrderLineID string  `json:"orderLineId"`
	Quantity    float64 `json:"quantity"`
}

type orderDetail struct {
	ID                  string    `json:"id"`
	Number              string    `json:"number"`
	Status              string    `json:"status"`
	PaymentStatus       string    `json:"paymentStatus"`
	PaymentInstructions *string   `json:"paymentInstructions"`
	Total               int64     `json:"total"`
	Subtotal            int64     `json:"subtotal"`
	ShippingAmount      int64     `json:"shippingAmount"`
	DiscountAmount      int64     `json:"discountAmount"`
	TaxAmount           int64     `json:"taxAmount"`
	AmountDue           int64     `json:"amountDue"`
	CurrencyCode        string    `json:"currencyCode"`
	CreatedAt           time.Time `json:"createdAt"`
	Carrier             string    `json:"carrier"`
	TrackingNumber      string    `json:"trackingNumber"`
	TrackingURL         *string   `json:"trackingUrl"`
	ShippingAddress     struct {
		City        string `json:"city"`
		CountryCode string `json:"countryCode"`
	} `json:"shippingAddress"`
	Lines  []orderLine `json:"lines"`
	Events []struct {
		Type      string    `json:"type"`
		Message   string    `json:"message"`
		CreatedAt time.Time `json:"createdAt"`
	} `json:"events"`
	Returns []struct {
		RmaNumber  string         `json:"rmaNumber"`
		Status     string         `json:"status"`
		Resolution string         `json:"resolution"`
		CreatedAt  time.Time      `json:"createdAt"`
		Lines      []lineQuantity `json:"lines"`
	} `json:"returns"`
	ReturnableLines []lineQuantity `json:"returnableLines"`
}

func orderTools() []ai.Tool {
	return []ai.Tool{{
		Name: "get_my_orders",
		Description: "The signed-in shopper's most recent orders on this store, newest first: number, date, status, total and tracking. " +
			"get_my_order has one order's items, delivery and returns.",
		Schema: schema(map[string]any{}),
		Effect: ai.Reads, Idempotent: true,
	}, {
		Name: "get_my_order",
		Description: "One of the signed-in shopper's orders, by its number: its items, status, payment, delivery and tracking, its returns, and what can still be sent back with the line ids start_return takes. " +
			"Puts a button to the order's page under your message.",
		Schema: schema(map[string]any{"order": str("The order number, as get_my_orders or the shopper gave it.")}, "order"),
		Effect: ai.Reads, Idempotent: true,
	}, {
		Name: "start_return",
		Description: "Ask the store to take back items from one of the shopper's shipped orders. Without confirmed it only checks what can go back and gives you a summary - nothing is sent. " +
			"Call it with confirmed true, and the same order, lines, reason and resolution, only after the shopper said yes to that summary in a later message. The store reviews every return.",
		Schema: schema(map[string]any{
			"order": str("The order number."),
			"lines": map[string]any{"type": "array", "minItems": 1, "maxItems": maxLinesPerUpdate, "description": "What goes back, from the returnable lines get_my_order lists.",
				"items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"line_id", "quantity"}, "properties": map[string]any{
					"line_id":  str("The line id get_my_order gave."),
					"quantity": map[string]any{"type": "number", "exclusiveMinimum": 0, "description": "How many of it go back."},
					"reason":   str("Why this item goes back, in the shopper's words, when it differs from the overall reason."),
				}}},
			"reason":     str("Why it's going back, in the shopper's words."),
			"resolution": map[string]any{"type": "string", "enum": []string{"refund", "exchange"}, "description": "Money back (refund, the default) or replacement goods (exchange)."},
			"confirmed":  map[string]any{"type": "boolean", "description": "True only once the shopper said yes to the summary the unconfirmed call gave."},
		}, "order", "lines"),
		Effect: ai.Writes,
	}}
}

func (c *conversation) myOrders(ctx context.Context, k call) ([]orderSummary, error) {
	if c.customer == nil {
		return nil, refusal(notSignedIn)
	}
	var out struct {
		MyOrders []orderSummary `json:"myOrders"`
	}
	if err := k.gql().Do(ctx, myOrdersDoc, nil, &out); err != nil {
		return nil, err
	}
	slices.SortStableFunc(out.MyOrders, func(a, b orderSummary) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out.MyOrders, nil
}

func (c *conversation) getMyOrders(ctx context.Context, k call) (string, error) {
	orders, err := c.myOrders(ctx, k)
	if err != nil {
		return "", err
	}
	if len(orders) == 0 {
		return "The shopper has no orders on this store yet.", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d orders, newest first:", len(orders))
	for _, o := range orders[:min(len(orders), maxOrdersListed)] {
		items := 0.0
		for _, l := range o.Lines {
			items += l.Quantity
		}
		fmt.Fprintf(&b, "\n- Order %s, placed %s: %s, %s, %s for %s item(s)", o.Number, day(o.CreatedAt), inWords(orderStatusWords, o.Status),
			inWords(paymentStatusWords, o.PaymentStatus), price(o.Total, o.CurrencyCode), qty(items))
		if o.TrackingNumber != "" {
			fmt.Fprintf(&b, ", tracking %s %s", o.Carrier, o.TrackingNumber)
		}
	}
	if len(orders) > maxOrdersListed {
		fmt.Fprintf(&b, "\n%d older orders aren't listed - ask for one by its number.", len(orders)-maxOrdersListed)
	}
	return b.String(), nil
}

// order finds one of the shopper's orders by the number they know it by, then reads it whole
func (c *conversation) order(ctx context.Context, k call) (orderDetail, error) {
	ref := strings.TrimPrefix(strings.TrimSpace(k.a.Order), "#")
	if ref == "" {
		return orderDetail{}, refusal("Say which order - its number, from get_my_orders or the shopper.")
	}
	orders, err := c.myOrders(ctx, k)
	if err != nil {
		return orderDetail{}, err
	}
	i := slices.IndexFunc(orders, func(o orderSummary) bool { return strings.EqualFold(o.Number, ref) || o.ID == ref })
	if i < 0 {
		return orderDetail{}, refusal(fmt.Sprintf("The shopper has no order %s on this store - get_my_orders lists theirs.", quote(ref)))
	}
	var out struct {
		MyOrder orderDetail `json:"myOrder"`
	}
	if err := k.gql().Do(ctx, myOrderDoc, map[string]any{"id": orders[i].ID}, &out); err != nil {
		return orderDetail{}, err
	}
	return out.MyOrder, nil
}

func (c *conversation) getMyOrder(ctx context.Context, k call) (string, error) {
	o, err := c.order(ctx, k)
	if err != nil {
		return "", err
	}
	c.orderButton(o)
	return renderOrder(o), nil
}

func renderOrder(o orderDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Order %s, placed %s: %s, %s.", o.Number, day(o.CreatedAt), inWords(orderStatusWords, o.Status), inWords(paymentStatusWords, o.PaymentStatus))
	if o.PaymentInstructions != nil && strings.TrimSpace(*o.PaymentInstructions) != "" {
		fmt.Fprintf(&b, "\nHow to pay: %s", clip(*o.PaymentInstructions, quoteChars))
	}
	b.WriteString("\nItems:")
	for _, l := range o.Lines {
		fmt.Fprintf(&b, "\n- %s × %s [sku %s, line %s] %s", qty(l.Quantity), l.Name, l.SKU, l.ID, price(l.TotalPrice, o.CurrencyCode))
		if l.IsGift {
			b.WriteString(" (free gift)")
		}
	}
	fmt.Fprintf(&b, "\nSubtotal %s, delivery %s", price(o.Subtotal, o.CurrencyCode), price(o.ShippingAmount, o.CurrencyCode))
	if o.DiscountAmount > 0 {
		fmt.Fprintf(&b, ", discount -%s", price(o.DiscountAmount, o.CurrencyCode))
	}
	if o.TaxAmount > 0 {
		fmt.Fprintf(&b, ", tax %s", price(o.TaxAmount, o.CurrencyCode))
	}
	fmt.Fprintf(&b, ", total %s", price(o.Total, o.CurrencyCode))
	if o.AmountDue > 0 && o.PaymentStatus == "UNPAID" {
		fmt.Fprintf(&b, ", still to pay %s", price(o.AmountDue, o.CurrencyCode))
	}
	if city := strings.TrimSpace(o.ShippingAddress.City); city != "" {
		fmt.Fprintf(&b, "\nGoing to %s, %s.", city, o.ShippingAddress.CountryCode)
	}
	switch {
	case o.TrackingNumber != "" && o.TrackingURL != nil && *o.TrackingURL != "":
		fmt.Fprintf(&b, "\nTracking: %s %s - %s", o.Carrier, o.TrackingNumber, *o.TrackingURL)
	case o.TrackingNumber != "":
		fmt.Fprintf(&b, "\nTracking: %s %s", o.Carrier, o.TrackingNumber)
	}
	if n := len(o.Events); n > 0 {
		b.WriteString("\nLatest updates:")
		for _, e := range o.Events[max(0, n-maxOrderEvents):] {
			fmt.Fprintf(&b, "\n- %s: %s", day(e.CreatedAt), clip(e.Message, quoteChars))
		}
	}
	names := map[string]string{}
	for _, l := range o.Lines {
		names[l.ID] = l.Name
	}
	for _, r := range o.Returns {
		parts := make([]string, len(r.Lines))
		for i, l := range r.Lines {
			parts[i] = qty(l.Quantity) + " × " + cmp.Or(names[l.OrderLineID], "an item")
		}
		fmt.Fprintf(&b, "\nReturn %s, opened %s for a %s: %s - %s.", r.RmaNumber, day(r.CreatedAt), r.Resolution, r.Status, strings.Join(parts, ", "))
	}
	switch returnable := returnableOf(o); {
	case len(returnable) > 0:
		b.WriteString("\nCan still be sent back:")
		for _, l := range o.Lines {
			if n := returnable[l.ID]; n > 0 {
				fmt.Fprintf(&b, "\n- up to %s × %s [line %s]", qty(n), l.Name, l.ID)
			}
		}
	case o.Status != "FULFILLED" && o.Status != "PARTIALLY_FULFILLED":
		b.WriteString("\nNothing can be sent back until it ships.")
	default:
		b.WriteString("\nNothing more can be sent back - it's all returned or in a return already.")
	}
	return b.String()
}

func returnableOf(o orderDetail) map[string]float64 {
	out := map[string]float64{}
	for _, l := range o.ReturnableLines {
		if l.Quantity > 0 {
			out[l.OrderLineID] = l.Quantity
		}
	}
	return out
}

func (c *conversation) orderButton(o orderDetail) {
	path := orderPagePath + o.ID
	if !slices.ContainsFunc(c.links, func(l Link) bool { return l.Path == path }) {
		c.links = append(c.links, Link{Label: "#" + strings.TrimPrefix(o.Number, "#"), Path: path}) // a number reads the same in every language
	}
}

// startReturn checks a return and summarises it, and sends it only once a later message of the shopper's followed that
// summary - the summary's key rides in the turn's recap, which the next message brings back
func (c *conversation) startReturn(ctx context.Context, k call) (string, error) {
	o, err := c.order(ctx, k)
	if err != nil {
		return "", err
	}
	resolution := cmp.Or(strings.TrimSpace(k.a.Resolution), resolutionRefund)
	if resolution != resolutionRefund && resolution != "exchange" {
		return "", refusal("resolution is refund or exchange.")
	}
	summary, err := returnSummary(o, k.a.Lines, resolution, strings.TrimSpace(k.a.Reason))
	if err != nil {
		return "", err
	}
	key := returnKey(o.ID, k.a.Lines, resolution, k.a.Reason)
	if !k.a.Confirmed {
		c.returns = append(c.returns, "Return "+key+" summarised for order "+o.Number+", not sent")
		return fmt.Sprintf("Return %s - nothing is sent yet. Confirmed, it asks the store to take back from order %s:\n%s\n"+
			"Tell the shopper exactly this and ask them to confirm. Once they say yes in a later message, call start_return again with the same order, lines, reason and resolution, and confirmed true.",
			key, o.Number, summary), nil
	}
	// the summary must be in the reply just before, so a yes can't confirm one from long ago
	latest := ""
	if n := len(c.earlier); n > 0 {
		latest = c.earlier[n-1]
	}
	open := alreadyOpen(o, k.a.Lines, resolution)
	switch {
	case slices.ContainsFunc(c.earlier, func(r string) bool { return strings.Contains(r, "Return "+key+" sent") }) || slices.Contains(c.sent, key):
		return "", refusal("That return was already sent - get_my_order shows it.")
	case open != "":
		return "", refusal(fmt.Sprintf("That return is already in as %s - get_my_order shows it.", open))
	case !strings.Contains(latest, "Return "+key+" summarised"):
		return "", refusal("The shopper hasn't confirmed this return. Call start_return without confirmed, tell them the summary and ask - " +
			"and only once they say yes, call it with confirmed. Anything changed since makes it a new summary to confirm.")
	}
	lines := make([]map[string]any, len(k.a.Lines))
	for i, l := range k.a.Lines {
		lines[i] = map[string]any{"orderLineId": strings.TrimSpace(l.LineID), "quantity": *l.Quantity, "reason": strings.TrimSpace(l.Reason)}
	}
	var out struct {
		RequestReturn struct {
			RmaNumber string `json:"rmaNumber"`
			Status    string `json:"status"`
		} `json:"requestReturn"`
	}
	input := map[string]any{"orderId": o.ID, "lines": lines, "reason": strings.TrimSpace(k.a.Reason), "resolution": resolution}
	if err := k.gql().Do(ctx, requestReturnDoc, map[string]any{"input": input}, &out); err != nil {
		return "", err
	}
	c.sent = append(c.sent, key)
	c.returns = append(c.returns, "Return "+key+" sent as "+out.RequestReturn.RmaNumber)
	c.orderButton(o)
	return fmt.Sprintf("Return %s is in for order %s, %s. The store reviews it and gets in touch about sending the items back - nothing is refunded or exchanged until then.",
		out.RequestReturn.RmaNumber, o.Number, out.RequestReturn.Status), nil
}

// alreadyOpen is a return the order already has for the same lines, opened in the last day and not turned down - one
// sent while the reply saying so got lost
// a return asked again within this is the same one, not a second
const returnRepeatWindow = 24 * time.Hour

func alreadyOpen(o orderDetail, lines []lineArg, resolution string) string {
	want := map[string]float64{}
	for _, l := range lines {
		if l.Quantity != nil {
			want[strings.TrimSpace(l.LineID)] += *l.Quantity
		}
	}
	for _, r := range o.Returns {
		if r.Status == "rejected" || r.Resolution != resolution || time.Since(r.CreatedAt) > returnRepeatWindow || len(r.Lines) != len(want) {
			continue
		}
		if !slices.ContainsFunc(r.Lines, func(l lineQuantity) bool { return want[l.OrderLineID] != l.Quantity }) {
			return r.RmaNumber
		}
	}
	return ""
}

func returnSummary(o orderDetail, lines []lineArg, resolution, reason string) (string, error) {
	returnable := returnableOf(o)
	if len(returnable) == 0 {
		if o.Status != "FULFILLED" && o.Status != "PARTIALLY_FULFILLED" {
			return "", refusal(fmt.Sprintf("Nothing on order %s can be sent back until it ships.", o.Number))
		}
		return "", refusal(fmt.Sprintf("Nothing more on order %s can be sent back - it's all returned or in a return already.", o.Number))
	}
	if len(lines) == 0 {
		return "", refusal("Say which lines go back, with the line ids get_my_order gave.")
	}
	byID := map[string]orderLine{}
	for _, l := range o.Lines {
		byID[l.ID] = l
	}
	var b strings.Builder
	seen := map[string]bool{}
	for _, l := range lines {
		id := strings.TrimSpace(l.LineID)
		line, ok := byID[id]
		switch {
		case !ok:
			return "", refusal(fmt.Sprintf("Order %s has no line %s - use the line ids get_my_order gave.", o.Number, quote(id)))
		case seen[id]:
			return "", refusal("Each line goes in once, with its whole quantity.")
		case l.Quantity == nil || *l.Quantity <= 0:
			return "", refusal("Each line needs a quantity above 0.")
		case *l.Quantity > returnable[id]:
			if returnable[id] == 0 {
				return "", refusal(fmt.Sprintf("%s can't be sent back - it hasn't shipped, or it's returned already.", line.Name))
			}
			return "", refusal(fmt.Sprintf("At most %s × %s can be sent back.", qty(returnable[id]), line.Name))
		}
		seen[id] = true
		fmt.Fprintf(&b, "- %s × %s (sku %s)", qty(*l.Quantity), line.Name, line.SKU)
		if why := strings.TrimSpace(l.Reason); why != "" {
			fmt.Fprintf(&b, ", because: %s", clip(why, maxReasonLen))
		}
		b.WriteString("\n")
	}
	if reason != "" {
		fmt.Fprintf(&b, "Reason: %s\n", clip(reason, maxReasonLen))
	}
	if resolution == resolutionRefund {
		b.WriteString("They'd like a refund, once the store has the items back.")
	} else {
		b.WriteString("They'd like it exchanged for the same items again.")
	}
	return b.String(), nil
}

// returnKey names one exact return - the same order, lines, quantities, reasons and resolution make the same key
func returnKey(orderID string, lines []lineArg, resolution, reason string) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		q := 0.0
		if l.Quantity != nil {
			q = *l.Quantity
		}
		parts[i] = strings.TrimSpace(l.LineID) + ":" + strconv.FormatFloat(q, 'f', -1, 64) + ":" + strings.Join(strings.Fields(l.Reason), " ")
	}
	slices.Sort(parts)
	sum := sha256.Sum256([]byte(orderID + "|" + strings.Join(parts, "|") + "|" + resolution + "|" + strings.Join(strings.Fields(reason), " ")))
	return "R" + hex.EncodeToString(sum[:4])
}

func recaps(msgs []Message) []string {
	var out []string
	for _, m := range msgs {
		if m.Role == "assistant" && m.Recap != "" {
			out = append(out, m.Recap)
		}
	}
	return out
}

func day(t time.Time) string { return t.UTC().Format("2 Jan 2006") }

func qty(q float64) string { return strconv.FormatFloat(q, 'f', -1, 64) }
