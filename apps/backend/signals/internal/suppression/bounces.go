package suppression

import (
	"context"
	"log"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

const (
	defaultSoftBounceThreshold = 3
	bounceDetailMax            = 240
)

// Bounces turns a send failure into a suppression signal when auto-suppress is on - a permanent 5xx rejection hard-suppresses the
// address, a transient one accrues toward the soft-bounce threshold, and timeouts or resets are ignored since they aren't a verdict
// on the address. The out-of-the-box feedback loop for pure-SMTP setups, shared by every sender - a provider webhook supplies richer signals
type Bounces struct {
	list *Service
	cfg  *config.Service
}

func NewBounces(list *Service, cfg *config.Service) *Bounces { return &Bounces{list: list, cfg: cfg} }

func (b *Bounces) Capture(ctx context.Context, addr string, sendErr error) {
	if b == nil || b.list == nil || b.cfg == nil || sendErr == nil {
		return
	}
	if on, _ := b.cfg.GetBool(ctx, "signals/deliverability/auto_suppress", config.DefaultScope); !on {
		return
	}
	permanent, matched := email.ClassifyBounce(sendErr)
	if !matched {
		return
	}
	detail := truncate(sendErr.Error(), bounceDetailMax)
	if permanent {
		if err := b.list.Suppress(ctx, addr, "hard_bounce", detail); err != nil {
			log.Printf("[suppression] suppress hard bounce for %s: %v", addr, err) // dropping it silently keeps mailing a dead address, which costs sender reputation
		}
		return
	}
	threshold := defaultSoftBounceThreshold
	if n, err := b.cfg.GetInt(ctx, "signals/deliverability/soft_bounce_threshold", config.DefaultScope); err == nil && n > 0 {
		threshold = int(n)
	}
	if _, _, err := b.list.RecordSoftBounce(ctx, addr, detail, threshold); err != nil {
		log.Printf("[suppression] record soft bounce for %s: %v", addr, err)
	}
}

// Cuts on a rune boundary - the detail is stored text, and a split rune would fail the insert that stops the next send
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
