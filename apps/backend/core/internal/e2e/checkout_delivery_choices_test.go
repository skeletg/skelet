package e2e

import (
	"context"
	"fmt"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// courierSlug is a second shipping agency, registered once for the package - external carriers seed disabled, so it quotes only on a channel that switches it on
const courierSlug = "e2e-courier"

type courierCarrier struct{}

func (courierCarrier) Quote(context.Context, shipping.RateRequest) ([]shipping.RatedMethod, error) {
	return []shipping.RatedMethod{{Code: "standard", Name: "Standard", PriceCents: 650, EstimatedDays: 2}}, nil
}

func init() {
	shipping.RegisterCarrier(shipping.CarrierRegistration{Slug: courierSlug, Carrier: courierCarrier{}, DisplayName: "E2E Courier"})
}

// storeTZ is far from UTC, so a store calendar read from the wrong place shows up on most hours of the day
const storeTZ = "Pacific/Kiritimati"

// choicesStore is a US store whose own rates offer Standard (5, 4 days), Express (12, next day, weekends too) and Economy (8, free from 100, 5 days), plus an inactive Slow method, with the e2e courier switched on alongside - and the default Delivery option adopted
func (h *harness) choicesStore(t *testing.T) *store {
	t.Helper()
	f := h.NewStore(t, 20, 2000)
	repo := h.D.Shipping.Repo()
	z, err := repo.CreateZone(h.Ctx, shipping.Zone{ChannelID: f.Channel.ID, Code: "e2e-us-" + uuid.NewString()[:8], Name: "US", IsActive: true})
	if err != nil {
		t.Fatalf("zone: %v", err)
	}
	if err := repo.AddCountryToZone(h.Ctx, z.ID, "US"); err != nil {
		t.Fatalf("zone country: %v", err)
	}
	for _, m := range []shipping.Method{
		{Code: "standard", Name: "Standard", CalcType: shipping.CalcFlat, FlatCents: 500, EstimatedDays: 4, SortOrder: 0, IsActive: true},
		{Code: "express", Name: "Express", CalcType: shipping.CalcFlat, FlatCents: 1200, EstimatedDays: 1, DeliversOnWeekends: true, SortOrder: 1, IsActive: true},
		{Code: "economy", Name: "Economy", CalcType: shipping.CalcFreeAbove, FlatCents: 800, FreeAboveCents: 10000, EstimatedDays: 5, SortOrder: 2, IsActive: true},
		{Code: "slow", Name: "Slow", CalcType: shipping.CalcFlat, FlatCents: 100, EstimatedDays: 9, SortOrder: 3, IsActive: false},
	} {
		m.ZoneID, m.CountryMode, m.DeliveryKind = z.ID, "all", delivery.KindShipToAddress
		if _, err := repo.CreateMethod(h.Ctx, m); err != nil {
			t.Fatalf("method %s: %v", m.Code, err)
		}
	}
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatalf("sync carriers: %v", err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, courierSlug, true); err != nil {
		t.Fatalf("courier: %v", err)
	}
	if _, err := h.D.Config.Set(h.Ctx, "general/store_information/timezone", storeTZ, config.Scope{Kind: config.ScopeChannel, ChannelID: &f.Channel.ID}); err != nil {
		t.Fatalf("timezone: %v", err)
	}
	if err := h.D.Delivery.AdoptDefaults(h.Ctx, f.Channel.ID, false); err != nil {
		t.Fatalf("adopt defaults: %v", err)
	}
	return f
}

// option finds the channel's saved option by name
func (h *harness) option(t *testing.T, f *store, name string) delivery.Option {
	t.Helper()
	opts, err := h.D.Delivery.ListOptions(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	for _, o := range opts {
		if o.Name == name {
			return o
		}
	}
	t.Fatalf("no option %q in %v", name, opts)
	return delivery.Option{}
}

// choice is one offered delivery as the storefront reads it
type choice struct {
	OptionID, OptionName, Carrier, Method string
	Cents                                 int
	Days                                  int
	Arrival                               string
}

func (c choice) key() string { return c.OptionName + " / " + c.Carrier + " / " + c.Method }

// gqlChoices reads the cart's single delivery group with its choices, estimates included
func (h *harness) gqlChoices(t *testing.T, ts *httptest.Server, f *store, cartID string) (string, []choice, map[string]any) {
	t.Helper()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ cart(id: %q) { checkout { deliveryGroups {
		key choices { optionId optionName kind carrierSlug methodCode priceCents estimatedDays estimatedArrival }
		selection { optionId optionName carrierSlug methodCode priceCents estimatedDays estimatedArrival }
		shipments { estimatedDays estimatedArrival } } } } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("cart groups: %v", errs)
	}
	groups := data["cart"].(map[string]any)["checkout"].(map[string]any)["deliveryGroups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want one", len(groups))
	}
	g := groups[0].(map[string]any)
	var out []choice
	for _, raw := range g["choices"].([]any) {
		c := raw.(map[string]any)
		if c["kind"] != delivery.KindShipToAddress {
			continue
		}
		ch := choice{OptionID: c["optionId"].(string), OptionName: c["optionName"].(string), Carrier: c["carrierSlug"].(string), Method: c["methodCode"].(string), Cents: int(c["priceCents"].(float64))}
		if d, ok := c["estimatedDays"].(float64); ok {
			ch.Days = int(d)
		}
		if a, ok := c["estimatedArrival"].(string); ok {
			ch.Arrival = a
		}
		out = append(out, ch)
	}
	sel, _ := g["selection"].(map[string]any)
	return g["key"].(string), out, sel
}

func keys(cs []choice) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.key()
	}
	return out
}

func find(t *testing.T, cs []choice, key string) choice {
	t.Helper()
	for _, c := range cs {
		if c.key() == key {
			return c
		}
	}
	t.Fatalf("no choice %q in %v", key, keys(cs))
	return choice{}
}

// arrival is the date the store's calendar gives a quote today
func arrival(t *testing.T, days int, weekends bool) string {
	t.Helper()
	loc, err := time.LoadLocation(storeTZ)
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	return shipping.ArrivalDate(time.Now().In(loc), days, weekends)
}

func (h *harness) choicesCart(t *testing.T, ts *httptest.Server, f *store, qty int) string {
	t.Helper()
	cartID := h.gqlGuestCart(t, ts, f, qty)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	return cartID
}

// Scenario: one door-delivery option offers every active method of every enabled carrier - the store's Standard, Express and Economy and the courier's Standard - each with its own price, days and arrival date; the inactive method is left out
func TestDeliveryChoicesOfferEveryMethodOfEveryCarrier(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	_, got, _ := h.gqlChoices(t, ts, f, h.choicesCart(t, ts, f, 1))
	want := []string{
		"Delivery / builtin / standard", "Delivery / builtin / express", "Delivery / builtin / economy", "Delivery / " + courierSlug + " / standard",
	}
	slices.Sort(want)
	have := keys(got)
	slices.Sort(have)
	if !slices.Equal(have, want) {
		t.Fatalf("choices = %v, want %v", have, want)
	}

	for _, w := range []struct {
		key      string
		cents    int
		days     int
		weekends bool
	}{
		{"Delivery / builtin / standard", 500, 4, false},
		{"Delivery / builtin / express", 1200, 1, true},
		{"Delivery / builtin / economy", 800, 5, false},
		{"Delivery / " + courierSlug + " / standard", 650, 2, false},
	} {
		c := find(t, got, w.key)
		if c.Cents != w.cents || c.Days != w.days {
			t.Errorf("%s: %d cents, %d days - want %d, %d", w.key, c.Cents, c.Days, w.cents, w.days)
		}
		if want := arrival(t, w.days, w.weekends); c.Arrival != want {
			t.Errorf("%s: arrives %s, want %s", w.key, c.Arrival, want)
		}
	}
}

// Scenario: Economy is free from 100 - below the threshold it costs its 8, at the threshold nothing, while Standard and Express still charge
func TestDeliveryChoicesWaiveFreeAboveAtTheThreshold(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	_, below, _ := h.gqlChoices(t, ts, f, h.choicesCart(t, ts, f, 4)) // 80
	if c := find(t, below, "Delivery / builtin / economy"); c.Cents != 800 {
		t.Errorf("below the threshold Economy = %d, want 800", c.Cents)
	}
	_, at, _ := h.gqlChoices(t, ts, f, h.choicesCart(t, ts, f, 5)) // 100
	if c := find(t, at, "Delivery / builtin / economy"); c.Cents != 0 {
		t.Errorf("at the threshold Economy = %d, want free", c.Cents)
	}
	if c := find(t, at, "Delivery / builtin / standard"); c.Cents != 500 {
		t.Errorf("Standard = %d, want 500 - only free_above waives", c.Cents)
	}
	if c := find(t, at, "Delivery / builtin / express"); c.Cents != 1200 {
		t.Errorf("Express = %d, want 1200", c.Cents)
	}
}

// Scenario: the courier's estimates skip weekends until the admin says it delivers on them, and the next quote counts Saturday and Sunday
func TestDeliveryChoicesFollowTheCarrierWeekendSetting(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.choicesCart(t, ts, f, 1)
	key := "Delivery / " + courierSlug + " / standard"

	_, before, _ := h.gqlChoices(t, ts, f, cartID)
	if c := find(t, before, key); c.Arrival != arrival(t, 2, false) {
		t.Errorf("weekday courier arrives %s, want %s", c.Arrival, arrival(t, 2, false))
	}
	if _, err := h.D.Shipping.SetCarrierWeekendDelivery(h.Ctx, f.Channel.ID, courierSlug, true); err != nil {
		t.Fatalf("weekend delivery: %v", err)
	}
	_, after, _ := h.gqlChoices(t, ts, f, cartID)
	if c := find(t, after, key); c.Arrival != arrival(t, 2, true) {
		t.Errorf("weekend courier arrives %s, want %s", c.Arrival, arrival(t, 2, true))
	}
	if _, err := h.D.Shipping.SetCarrierWeekendDelivery(h.Ctx, f.Channel.ID, shipping.BuiltinCarrierSlug, true); err == nil {
		t.Error("the built-in carrier took a channel-wide weekend setting - its methods set their own")
	}
}

// Scenario: a carrier that ships only to Canada, or one switched off, offers nothing to a US cart
func TestDeliveryChoicesLeaveOutCarriersThatCannotShip(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.choicesCart(t, ts, f, 1)
	courier := "Delivery / " + courierSlug + " / standard"

	if err := h.D.Shipping.SetCarrierCountries(h.Ctx, f.Channel.ID, courierSlug, "specific", []string{"CA"}); err != nil {
		t.Fatalf("courier countries: %v", err)
	}
	_, got, _ := h.gqlChoices(t, ts, f, cartID)
	if slices.Contains(keys(got), courier) {
		t.Errorf("a Canada-only courier quoted a US cart: %v", keys(got))
	}

	if err := h.D.Shipping.SetCarrierCountries(h.Ctx, f.Channel.ID, courierSlug, "all", nil); err != nil {
		t.Fatalf("courier countries: %v", err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, courierSlug, false); err != nil {
		t.Fatalf("disable courier: %v", err)
	}
	_, got, _ = h.gqlChoices(t, ts, f, cartID)
	if slices.Contains(keys(got), courier) {
		t.Errorf("a disabled courier quoted: %v", keys(got))
	}
	if len(got) != 3 {
		t.Errorf("choices = %v, want the store's three", keys(got))
	}
}

// Scenario: two door options each limited to an agency list only that agency's methods - no method appears twice, and neither option is warned about overlap
func TestDeliveryOptionsSplitByCarrier(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	store := h.option(t, f, "Delivery")
	if _, err := h.D.Delivery.UpdateOption(h.Ctx, store.ID, delivery.SaveOptionInput{
		Kind: store.Kind, Name: "Store courier", Enabled: true, BlockModes: store.BlockModes, CarrierSlug: shipping.BuiltinCarrierSlug,
	}); err != nil {
		t.Fatalf("limit option: %v", err)
	}
	if _, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{
		Kind: delivery.KindShipToAddress, Name: "Courier", Enabled: true, CarrierSlug: courierSlug, SortOrder: 5,
	}); err != nil {
		t.Fatalf("courier option: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	_, got, _ := h.gqlChoices(t, ts, f, h.choicesCart(t, ts, f, 1))
	// Options in their order, and within one its methods by carrier, then price
	want := []string{
		"Store courier / builtin / standard", "Store courier / builtin / economy", "Store courier / builtin / express",
		"Courier / " + courierSlug + " / standard",
	}
	if have := keys(got); !slices.Equal(have, want) {
		t.Fatalf("choices = %v, want %v", have, want)
	}
	for _, name := range []string{"Store courier", "Courier"} {
		if c := overlapOf(h, t, f, name); !c.OK {
			t.Errorf("%s warned of an overlap: %s", name, c.Detail)
		}
	}
}

// Scenario: a second door option with no carrier lists every method again - the shopper sees each twice - and both options carry the overlap warning while staying enabled
func TestDeliveryOptionsOfOneKindWithoutACarrierOverlap(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	if _, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{
		Kind: delivery.KindShipToAddress, Name: "Home delivery", Enabled: true, SortOrder: 5,
	}); err != nil {
		t.Fatalf("second option: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	_, got, _ := h.gqlChoices(t, ts, f, h.choicesCart(t, ts, f, 1))
	for _, method := range []string{"builtin / standard", "builtin / express", "builtin / economy", courierSlug + " / standard"} {
		for _, opt := range []string{"Delivery", "Home delivery"} {
			find(t, got, opt+" / "+method)
		}
	}
	if len(got) != 8 {
		t.Errorf("choices = %d, want every method under each option: %v", len(got), keys(got))
	}
	for _, name := range []string{"Delivery", "Home delivery"} {
		o := h.option(t, f, name)
		if !o.Enabled {
			t.Errorf("%s was switched off by an advisory check", name)
		}
		if c := overlapOf(h, t, f, name); c.OK || !c.Advisory {
			t.Errorf("%s: overlap check = %+v, want a failing advisory", name, c)
		}
	}
}

func overlapOf(h *harness, t *testing.T, f *store, name string) delivery.Check {
	t.Helper()
	for _, c := range h.D.Delivery.Readiness(h.Ctx, f.Channel.ID, h.option(t, f, name)) {
		if c.Code == "overlap" {
			return c
		}
	}
	t.Fatalf("%s has no overlap check", name)
	return delivery.Check{}
}

// Scenario: with two door options, the setCartShippingMethod shorthand binds the first; setCartDelivery picks the second by id, and the placed order records that option, carrier, method, price and estimate
func TestDeliveryChoiceOfASecondOptionReachesTheOrder(t *testing.T) {
	h := newHarness(t)
	f := h.choicesStore(t)
	if _, err := h.D.Delivery.CreateOption(h.Ctx, f.Channel.ID, delivery.SaveOptionInput{
		Kind: delivery.KindShipToAddress, Name: "Express lane", Enabled: true, SortOrder: 5,
	}); err != nil {
		t.Fatalf("second option: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.choicesCart(t, ts, f, 1)

	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "express")`, cartID))
	groupKey, got, sel := h.gqlChoices(t, ts, f, cartID)
	if sel == nil || sel["optionName"] != "Delivery" || sel["methodCode"] != "express" {
		t.Fatalf("shorthand selection = %v, want Express under the first option, Delivery", sel)
	}

	lane := find(t, got, "Express lane / builtin / express")
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDelivery(cartId: %q, groupKey: %q, optionId: %q, carrierSlug: "builtin", methodCode: "express")`, cartID, groupKey, lane.OptionID))
	_, _, sel = h.gqlChoices(t, ts, f, cartID)
	if sel == nil || sel["optionId"] != lane.OptionID || sel["estimatedArrival"] != arrival(t, 1, true) {
		t.Fatalf("selection = %v, want Express lane arriving %s", sel, arrival(t, 1, true))
	}

	email := "choices-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	co := h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	if got := co["totals"].(map[string]any)["shippingCents"].(float64); got != 1200 {
		t.Fatalf("shipping = %v, want Express's 1200", got)
	}
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	orderID := uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string))
	ships, err := h.D.Queries.ListOrderShipments(h.Ctx, orderID)
	if err != nil || len(ships) != 1 {
		t.Fatalf("shipments = %d, %v - want one", len(ships), err)
	}
	s := ships[0]
	if s.DeliveryOptionName != "Express lane" || s.CarrierSlug != "builtin" || s.MethodCode != "express" || s.AmountCents != 1200 ||
		!s.EstimatedDays.Valid || s.EstimatedDays.Int32 != 1 {
		t.Errorf("shipment = %s/%s/%s %d cents, %v days - want Express lane/builtin/express 1200, 1",
			s.DeliveryOptionName, s.CarrierSlug, s.MethodCode, s.AmountCents, s.EstimatedDays)
	}
	if !s.DeliveryOptionID.Valid || s.DeliveryOptionID.UUID.String() != lane.OptionID {
		t.Errorf("shipment option = %v, want %s", s.DeliveryOptionID, lane.OptionID)
	}
	h.Settle(t)
}
