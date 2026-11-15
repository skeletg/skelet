package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestNumberFormats(t *testing.T) {
	at := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	for format, want := range map[string]string{
		"ORD-{seq:6}":              "ORD-000042",
		"{channel}-{yyyy}-{seq:4}": "outlet-2026-0042",
		"{yy}{mm}/{seq}":           "2603/42",
		"#{seq}":                   "#42",
		"INV.{yyyy}_{seq:12}":      "INV.2026_000000000042",
	} {
		f, err := parseNumberFormat(format)
		if err != nil {
			t.Errorf("%s: %v", format, err)
			continue
		}
		if got := f.render(42, at, "outlet"); got != want {
			t.Errorf("%s renders %q, want %q", format, got, want)
		}
	}
	for format, why := range map[string]string{
		"ORD-{yyyy}":     "needs {seq}",
		"ORD {seq}":      "only letters",
		"ORD-{day}{seq}": "isn't a placeholder",
		"{yyyy:2}{seq}":  "only {seq} takes a width",
		"{seq:13}":       "at most 12",
		"{seq}}":         "only letters",
	} {
		if _, err := parseNumberFormat(format); err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("%s: %v, want an error about %q", format, err, why)
		}
	}
	if checkNumberFormat("  ") != nil || checkNumberFormat("x") == nil {
		t.Error("an empty format goes back to the default, and one without {seq} is refused")
	}
}

type fakeSettings map[string]string

func (f fakeSettings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f[path], nil
}

type counts map[string]int64

func (c counts) next(_ context.Context, scope, period string) (int64, error) {
	c[scope+"|"+period]++
	return c[scope+"|"+period], nil
}

func TestNumberingFollowsTheChannelsSettings(t *testing.T) {
	ctx := context.Background()
	ch := channel.Channel{ID: uuid.New(), Slug: "outlet"}
	newYearsEve := time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	number := func(s fakeSettings, c counts, at time.Time) string {
		t.Helper()
		got, err := NewNumbering(s, c.next, nil).Number(ctx, ch, at)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	if got := number(fakeSettings{}, counts{}, newYearsEve); got != "ORD-000001" {
		t.Errorf("nothing set: %q, want the default format", got)
	}
	c := counts{}
	s := fakeSettings{numberFormatPath: "{channel}-{yyyy}-{seq:3}", numberCountPath: "channel", numberResetPath: "yearly",
		"general/store_information/timezone": "Europe/Berlin"}
	if got := number(s, c, newYearsEve); got != "outlet-2027-001" || c[ch.ID.String()+"|2027"] != 1 {
		t.Errorf("a channel's yearly count, in its timezone: %q, counts %v", got, c)
	}

	c = counts{}
	if got := number(fakeSettings{numberFormatPath: "S-{seq}", numberCountPath: "channel", numberResetPath: "yearly"}, c, newYearsEve); got != "S-1" || c["|"] != 1 {
		t.Errorf("unsafe settings: %q, counts %v - want the store's count, never restarting", got, c)
	}
	if got := number(fakeSettings{numberFormatPath: "bad format"}, counts{}, newYearsEve); got != "ORD-000001" {
		t.Errorf("an unusable saved format: %q, want the default", got)
	}
}

func TestAPluginCanNameOrders(t *testing.T) {
	ctx := context.Background()
	ch := channel.Channel{ID: uuid.New(), Slug: "outlet"}
	c := counts{}
	plugin := func(answer string, err error) func(context.Context, NumberRequest) (string, bool, error) {
		return func(_ context.Context, req NumberRequest) (string, bool, error) {
			n, _ := req.Next("acme", "")
			if answer == "" && err == nil {
				return fmt.Sprintf("ACME-%d", n), true, nil
			}
			return answer, true, err
		}
	}
	got, err := NewNumbering(fakeSettings{}, c.next, plugin("", nil)).Number(ctx, ch, time.Now())
	if err != nil || got != "ACME-1" || c["acme|"] != 1 {
		t.Errorf("a plugin's number: %q, %v, counts %v", got, err, c)
	}
	for _, p := range []func(context.Context, NumberRequest) (string, bool, error){plugin("", errors.New("down")), plugin("has space", nil)} {
		if got, _ := NewNumbering(fakeSettings{}, c.next, p).Number(ctx, ch, time.Now()); !strings.HasPrefix(got, "ORD-") {
			t.Errorf("a plugin that failed or answered nonsense: %q, want the store's format", got)
		}
	}
	none := func(context.Context, NumberRequest) (string, bool, error) { return "", false, nil }
	if got, _ := NewNumbering(fakeSettings{}, c.next, none).Number(ctx, ch, time.Now()); !strings.HasPrefix(got, "ORD-") {
		t.Errorf("no plugin on the channel: %q", got)
	}
}
