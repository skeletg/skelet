package subscription

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/syncveto"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const (
	EventRenewing  = "subscription.renewing"
	retryDaysPath  = "sales/subscriptions/retry_days"
	maxRetries     = 10
	maxRetryDays   = 60
	defaultRetries = "1, 3, 7"
	// The most a hook can make one period cost, in minor units - far past any real renewal, short of a total that overflows
	MaxRenewalCents = 10_000_000_000
)

func init() {
	config.RegisterGroup(config.Group{Section: "sales", Slug: "subscriptions", Label: "Subscriptions", SortOrder: 5, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: retryDaysPath, Type: config.TypeString, Default: defaultRetries, Label: "Retry a failed renewal after (days)",
		Description: "The days to wait before each retry of a renewal charge that failed, in order - 1, 3, 7 tries again a day later, then three days after that, then a week after that, and cancels the subscription when the last one fails. 0 cancels at the first failure, and empty uses 1, 3, 7.",
		Check:       func(v string) error { _, err := parseRetryDays(v); return err }})
}

func parseRetryDays(v string) ([]time.Duration, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "0" {
		return nil, nil
	}
	var out []time.Duration
	for part := range strings.SplitSeq(v, ",") {
		days, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || days < 1 || days > maxRetryDays {
			return nil, fmt.Errorf("%q: each retry waits a whole number of days from 1 to %d", strings.TrimSpace(part), maxRetryDays)
		}
		out = append(out, time.Duration(days)*24*time.Hour)
	}
	if len(out) > maxRetries {
		return nil, fmt.Errorf("at most %d retries", maxRetries)
	}
	return out, nil
}

type Settings interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

func (s *Service) SetSettings(settings Settings) { s.settings = settings }

// retryLadder is the channel's retry schedule - the built-in one when nothing readable is set
func (s *Service) retryLadder(ctx context.Context, sub Subscription) []time.Duration {
	if s.settings == nil {
		return dunningBackoff
	}
	// A cleared field means the default, never "no retries" - "0" says that
	scope := config.DefaultScope
	if sub.ChannelID != nil {
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: sub.ChannelID}
	}
	v, err := s.settings.GetString(ctx, retryDaysPath, scope)
	if err != nil {
		return dunningBackoff
	}
	ladder, err := parseRetryDays(cmp.Or(strings.TrimSpace(v), defaultRetries))
	if err != nil {
		return dunningBackoff
	}
	return ladder
}

// Fires BEFORE a period is invoiced, ahead of the billing lock, and may come again for the same period when the plan
// is thrown away - sync handlers set the amount, skip the period or move it later - Response {reject, reason,
// amountCents, skip, delayUntil}
type Renewing struct {
	SubscriptionID uuid.UUID
	ChannelID      uuid.UUID
	CustomerID     *uuid.UUID
	ProductID      uuid.UUID
	SKU            string
	Quantity       float64
	Sequence       int32
	PeriodStart    time.Time
	PeriodEnd      time.Time
	CurrencyCode   string
	AmountCents    int64
	Skip           bool
	DelayUntil     *time.Time
	now            time.Time
}

func (*Renewing) Type() string                { return EventRenewing }
func (r *Renewing) EventChannelID() uuid.UUID { return r.ChannelID }

func (r *Renewing) ApplyWebhookResponse(body []byte) (bool, string, error) {
	if rejected, reason, err := syncveto.Decode(body); err != nil || rejected {
		return rejected, reason, err
	}
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		AmountCents *int64     `json:"amountCents"`
		Skip        bool       `json:"skip"`
		DelayUntil  *time.Time `json:"delayUntil"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.AmountCents != nil {
		if err := r.SetRenewalAmount(*resp.AmountCents); err != nil {
			return false, "", err
		}
	}
	if resp.Skip {
		r.SkipRenewal()
	}
	if resp.DelayUntil != nil {
		if err := r.DelayRenewal(*resp.DelayUntil); err != nil {
			return false, "", err
		}
	}
	return false, "", nil
}

func (r *Renewing) RenewalSubscriptionID() uuid.UUID { return r.SubscriptionID }
func (r *Renewing) RenewalChannelID() uuid.UUID      { return r.ChannelID }
func (r *Renewing) RenewalCustomerID() *uuid.UUID    { return r.CustomerID }
func (r *Renewing) RenewalProductID() uuid.UUID      { return r.ProductID }
func (r *Renewing) RenewalSKU() string               { return r.SKU }
func (r *Renewing) RenewalQuantity() float64         { return r.Quantity }
func (r *Renewing) RenewalSequence() int             { return int(r.Sequence) }
func (r *Renewing) RenewalPeriodStart() time.Time    { return r.PeriodStart }
func (r *Renewing) RenewalPeriodEnd() time.Time      { return r.PeriodEnd }
func (r *Renewing) RenewalCurrencyCode() string      { return r.CurrencyCode }
func (r *Renewing) RenewalAmountCents() int64        { return r.AmountCents }
func (r *Renewing) SkipRenewal()                     { r.Skip = true }

func (r *Renewing) SetRenewalAmount(cents int64) error {
	switch {
	case cents < 0:
		return errors.New("a renewal can't cost less than nothing")
	case cents > MaxRenewalCents:
		return fmt.Errorf("a renewal costs at most %d in minor units", MaxRenewalCents)
	}
	r.AmountCents = cents
	return nil
}

func (r *Renewing) DelayRenewal(until time.Time) error {
	if !until.After(r.now) {
		return errors.New("a renewal can only move later than now")
	}
	until = until.UTC()
	r.DelayUntil = &until
	return nil
}
