package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Scenario: a channel's own format, count and yearly restart name its orders, while a channel left alone keeps the store's count
func TestOrderNumbersFollowEachChannelsFormat(t *testing.T) {
	h := newHarness(t)
	own, plain := h.NewStore(t, 10, 2000), h.NewStore(t, 10, 2000)
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &own.Channel.ID}
	for path, v := range map[string]string{
		"sales/orders/number_format": "{channel}-{yyyy}-{seq:4}",
		"sales/orders/number_count":  "channel",
		"sales/orders/number_reset":  "yearly",
	} {
		if _, err := h.D.Config.Set(h.Ctx, path, v, scope); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	if _, err := h.D.Config.Set(h.Ctx, "sales/orders/number_format", "no-count", scope); err == nil {
		t.Error("a format without {seq} was saved")
	}

	year := time.Now().UTC().Year()
	for i, want := range []string{"%s-%d-0001", "%s-%d-0002"} {
		if got := h.PlaceGuestCOD(t, own, 1).Number; got != fmt.Sprintf(want, strings.ToUpper(own.Channel.Slug), year) {
			t.Errorf("order %d on the channel with its own format = %q", i+1, got)
		}
	}
	if got := h.PlaceGuestCOD(t, plain, 1).Number; !regexp.MustCompile(`^ORD-\d{6,}$`).MatchString(got) {
		t.Errorf("the channel left alone = %q, want the store's format", got)
	}

	// The count restarted under a format that once gave these numbers out skips past them
	if _, err := h.D.Config.Set(h.Ctx, "sales/orders/number_reset", "never", scope); err != nil {
		t.Fatal(err)
	}
	if got := h.PlaceGuestCOD(t, own, 1).Number; got != fmt.Sprintf("%s-%d-0003", strings.ToUpper(own.Channel.Slug), year) {
		t.Errorf("a count that would reuse a number = %q", got)
	}
}
