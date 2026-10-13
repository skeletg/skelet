package front

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

// A Friday, so weekend rules show in the arrival dates
var choicesToday = time.Date(2026, time.October, 2, 10, 0, 0, 0, time.UTC)

func quote(carrier, code, kind string, cents int64, days int, weekends bool) shipping.RatedQuote {
	return shipping.RatedQuote{CarrierSlug: carrier, CarrierName: carrier + " Co", Method: shipping.RatedMethod{
		Code: code, Name: code, PriceCents: cents, EstimatedDays: days, Kind: kind, DeliversOnWeekends: weekends,
	}}
}

func deliveryOpt(name, kind, carrier string) delivery.Option {
	return delivery.Option{ID: uuid.New(), Name: name, Kind: kind, CarrierSlug: carrier, Enabled: true}
}

// What a choice is, for comparing - option name, carrier, method, price, days, arrival
type choiceRow struct {
	option, carrier, method string
	cents                   int64
	days                    int
	arrival                 string
}

func rows(chs []deliveryChoice) []choiceRow {
	out := make([]choiceRow, 0, len(chs))
	for _, c := range chs {
		out = append(out, choiceRow{c.option.Name, c.carrierSlug, c.methodCode, c.priceCents, c.estimatedDays, c.estimatedArrival})
	}
	return out
}

func TestMatchChoices(t *testing.T) {
	door := []string{delivery.KindShipToAddress}
	std := quote("builtin", "standard", delivery.KindShipToAddress, 500, 4, false)
	express := quote("builtin", "express", delivery.KindShipToAddress, 1200, 1, true)
	free := quote("builtin", "free", delivery.KindShipToAddress, 0, 5, false)
	dhl := quote("dhl", "standard", delivery.KindShipToAddress, 650, 2, false)
	locker := quote("builtin", "locker", delivery.KindParcelLocker, 300, 2, false)

	cases := []struct {
		name        string
		options     []delivery.Option
		kinds       []string
		perShipment [][]shipping.RatedQuote
		want        []choiceRow
	}{
		{
			name:        "one door option offers standard, express and free with their arrivals",
			options:     []delivery.Option{deliveryOpt("Delivery", delivery.KindShipToAddress, "")},
			kinds:       door,
			perShipment: [][]shipping.RatedQuote{{std, express, free}},
			want: []choiceRow{
				{"Delivery", "builtin", "standard", 500, 4, "2026-10-08"},
				{"Delivery", "builtin", "express", 1200, 1, "2026-10-03"},
				{"Delivery", "builtin", "free", 0, 5, "2026-10-09"},
			},
		},
		{
			name:        "every carrier's methods of the kind are offered, a same-coded method kept apart by carrier",
			options:     []delivery.Option{deliveryOpt("Delivery", delivery.KindShipToAddress, "")},
			kinds:       door,
			perShipment: [][]shipping.RatedQuote{{std, dhl}},
			want: []choiceRow{
				{"Delivery", "builtin", "standard", 500, 4, "2026-10-08"},
				{"Delivery", "dhl", "standard", 650, 2, "2026-10-06"},
			},
		},
		{
			name: "options limited to a carrier each list only that carrier",
			options: []delivery.Option{
				deliveryOpt("Store courier", delivery.KindShipToAddress, "builtin"),
				deliveryOpt("DHL", delivery.KindShipToAddress, "dhl"),
			},
			kinds:       door,
			perShipment: [][]shipping.RatedQuote{{std, express, dhl}},
			want: []choiceRow{
				{"Store courier", "builtin", "standard", 500, 4, "2026-10-08"},
				{"Store courier", "builtin", "express", 1200, 1, "2026-10-03"},
				{"DHL", "dhl", "standard", 650, 2, "2026-10-06"},
			},
		},
		{
			name: "two options of a kind without a carrier each list every method",
			options: []delivery.Option{
				deliveryOpt("Delivery", delivery.KindShipToAddress, ""),
				deliveryOpt("Home delivery", delivery.KindShipToAddress, ""),
			},
			kinds:       door,
			perShipment: [][]shipping.RatedQuote{{std}},
			want: []choiceRow{
				{"Delivery", "builtin", "standard", 500, 4, "2026-10-08"},
				{"Home delivery", "builtin", "standard", 500, 4, "2026-10-08"},
			},
		},
		{
			name: "a method only goes to options of its kind",
			options: []delivery.Option{
				deliveryOpt("Delivery", delivery.KindShipToAddress, ""),
				deliveryOpt("Locker", delivery.KindParcelLocker, ""),
			},
			kinds:       []string{delivery.KindShipToAddress, delivery.KindParcelLocker},
			perShipment: [][]shipping.RatedQuote{{std, locker}},
			want: []choiceRow{
				{"Delivery", "builtin", "standard", 500, 4, "2026-10-08"},
				{"Locker", "builtin", "locker", 300, 2, "2026-10-06"},
			},
		},
		{
			name: "an option of a kind the group can't take is skipped",
			options: []delivery.Option{
				deliveryOpt("Delivery", delivery.KindShipToAddress, ""),
				deliveryOpt("Locker", delivery.KindParcelLocker, ""),
			},
			kinds:       door,
			perShipment: [][]shipping.RatedQuote{{std, locker}},
			want:        []choiceRow{{"Delivery", "builtin", "standard", 500, 4, "2026-10-08"}},
		},
		{
			name:    "several shipments sum their prices, take the slowest days, and drop a method one can't take",
			options: []delivery.Option{deliveryOpt("Delivery", delivery.KindShipToAddress, "")},
			kinds:   door,
			perShipment: [][]shipping.RatedQuote{
				{std, express},
				{quote("builtin", "standard", delivery.KindShipToAddress, 300, 6, false)},
			},
			want: []choiceRow{{"Delivery", "builtin", "standard", 800, 6, "2026-10-12"}},
		},
		{
			name:        "a method with no estimate has no arrival",
			options:     []delivery.Option{deliveryOpt("Delivery", delivery.KindShipToAddress, "")},
			kinds:       door,
			perShipment: [][]shipping.RatedQuote{{quote("builtin", "standard", delivery.KindShipToAddress, 500, 0, true)}},
			want:        []choiceRow{{"Delivery", "builtin", "standard", 500, 0, ""}},
		},
		{
			name:        "an unregistered kind offers nothing",
			options:     []delivery.Option{deliveryOpt("Drone", "drone", "")},
			kinds:       []string{"drone"},
			perShipment: [][]shipping.RatedQuote{{quote("builtin", "drone", "drone", 100, 1, false)}},
			want:        []choiceRow{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rows(matchChoices(c.options, c.kinds, c.perShipment, choicesToday)); !slices.Equal(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// Each shipment's own price is kept, so the order records what every package cost
func TestMatchChoicesKeepsPerShipmentPrices(t *testing.T) {
	got := matchChoices(
		[]delivery.Option{deliveryOpt("Delivery", delivery.KindShipToAddress, "")},
		[]string{delivery.KindShipToAddress},
		[][]shipping.RatedQuote{
			{quote("builtin", "standard", delivery.KindShipToAddress, 500, 2, false)},
			{quote("builtin", "standard", delivery.KindShipToAddress, 300, 3, false)},
		},
		choicesToday,
	)
	if len(got) != 1 || !slices.Equal(got[0].perShipment, []int64{500, 300}) {
		t.Fatalf("got %+v", got)
	}
}
