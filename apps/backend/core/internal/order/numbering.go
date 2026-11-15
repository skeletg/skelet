package order

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const (
	numberFormatPath    = "sales/orders/number_format"
	numberCountPath     = "sales/orders/number_count"
	numberResetPath     = "sales/orders/number_reset"
	DefaultNumberFormat = "ORD-{seq:6}"
	maxNumberFormat     = 64
)

func init() {
	config.Register(config.Definition{
		Path: numberFormatPath, Type: config.TypeString, Default: DefaultNumberFormat, Label: "Order number format",
		Description: "How a new order's number reads. {seq} is the count, {seq:6} the count padded to six digits, {yyyy} {yy} and {mm} the date it was placed and {channel} the channel's slug, with letters, digits and - _ . / # around them. Orders already placed keep their number.",
		MaxLen:      maxNumberFormat, Check: checkNumberFormat,
	})
	config.Register(config.Definition{
		Path: numberCountPath, Type: config.TypeSelect, Default: "store", Label: "Order number count",
		Description: "One count runs across every channel, or each channel counts its own - which needs {channel} in the format, so two channels never give out the same number.",
		Options:     []config.Option{{Value: "store", Label: "One for the whole store"}, {Value: "channel", Label: "One per channel"}},
	})
	config.Register(config.Definition{
		Path: numberResetPath, Type: config.TypeSelect, Default: "never", Label: "Start the count again",
		Description: "Every year starts the count at 1 on January 1st in the store's timezone - which needs {yyyy} or {yy} in the format, so a number is never given out twice.",
		Options:     []config.Option{{Value: "never", Label: "Never"}, {Value: "yearly", Label: "Every year"}},
	})
}

type numberPart struct {
	text, token string
	width       int
}

type numberFormat []numberPart

var (
	numberToken   = regexp.MustCompile(`\{([a-z]+)(?::(\d{1,2}))?\}`)
	numberLiteral = regexp.MustCompile(`^[A-Za-z0-9_./#-]*$`)
	usableNumber  = regexp.MustCompile(`^[A-Za-z0-9_./#-]{1,64}$`)
)

func parseNumberFormat(s string) (numberFormat, error) {
	var out numberFormat
	literal := func(t string) error {
		if !numberLiteral.MatchString(t) {
			return fmt.Errorf("%q: only letters, digits and - _ . / # go around the placeholders", t)
		}
		if t != "" {
			out = append(out, numberPart{text: t})
		}
		return nil
	}
	last, counted := 0, false
	for _, m := range numberToken.FindAllStringSubmatchIndex(s, -1) {
		if err := literal(s[last:m[0]]); err != nil {
			return nil, err
		}
		token, width := s[m[2]:m[3]], 0
		if m[4] >= 0 {
			width, _ = strconv.Atoi(s[m[4]:m[5]])
		}
		switch {
		case token == "seq" && width > 12:
			return nil, errors.New("{seq} pads to at most 12 digits")
		case token == "seq":
			counted = true
		case token != "yyyy" && token != "yy" && token != "mm" && token != "channel":
			return nil, fmt.Errorf("{%s} isn't a placeholder - {seq}, {yyyy}, {yy}, {mm} and {channel} are", token)
		case m[4] >= 0:
			return nil, fmt.Errorf("only {seq} takes a width, not {%s}", token)
		}
		out = append(out, numberPart{token: token, width: width})
		last = m[1]
	}
	if err := literal(s[last:]); err != nil {
		return nil, err
	}
	if !counted {
		return nil, errors.New("the format needs {seq}, or two orders could share a number")
	}
	return out, nil
}

func checkNumberFormat(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	_, err := parseNumberFormat(strings.TrimSpace(value))
	return err
}

func (f numberFormat) has(tokens ...string) bool {
	return slices.ContainsFunc(f, func(p numberPart) bool { return slices.Contains(tokens, p.token) })
}

func (f numberFormat) render(seq int64, at time.Time, channelSlug string) string {
	var b strings.Builder
	for _, p := range f {
		switch p.token {
		case "":
			b.WriteString(p.text)
		case "seq":
			fmt.Fprintf(&b, "%0*d", p.width, seq)
		case "yyyy":
			fmt.Fprintf(&b, "%04d", at.Year())
		case "yy":
			fmt.Fprintf(&b, "%02d", at.Year()%100)
		case "mm":
			fmt.Fprintf(&b, "%02d", int(at.Month()))
		case "channel":
			b.WriteString(channelSlug)
		}
	}
	return b.String()
}

// NumberRequest is what a plugin's numberer gets - Next draws from core's count for a scope and period it names
type NumberRequest struct {
	ChannelID   uuid.UUID
	ChannelSlug string
	At          time.Time
	Next        func(scope, period string) (int64, error)
}

type NumberSettings interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

// Numbering names new orders - a plugin's numberer when one is on for the channel, else the channel's format
type Numbering struct {
	settings NumberSettings
	next     func(ctx context.Context, scope, period string) (int64, error)
	plugin   func(ctx context.Context, req NumberRequest) (string, bool, error)
}

func NewNumbering(settings NumberSettings, next func(ctx context.Context, scope, period string) (int64, error), plugin func(ctx context.Context, req NumberRequest) (string, bool, error)) *Numbering {
	return &Numbering{settings: settings, next: next, plugin: plugin}
}

func (n *Numbering) setting(ctx context.Context, path string, scope config.Scope) string {
	if n.settings == nil {
		return ""
	}
	v, err := n.settings.GetString(ctx, path, scope)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func (n *Numbering) Number(ctx context.Context, ch channel.Channel, now time.Time) (string, error) {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
	at := now
	if loc, err := time.LoadLocation(n.setting(ctx, "general/store_information/timezone", scope)); err == nil {
		at = now.In(loc)
	}
	log := applog.From(ctx)
	if n.plugin != nil {
		num, ok, err := n.plugin(ctx, NumberRequest{ChannelID: ch.ID, ChannelSlug: ch.Slug, At: at, Next: func(scope, period string) (int64, error) {
			return n.next(ctx, scope, period)
		}})
		switch {
		case ok && err == nil && usableNumber.MatchString(num):
			return num, nil
		case ok:
			log.Warn("order: a plugin's order number is unusable, the store's format names the order", "number", num, "err", err)
		}
	}
	f, err := parseNumberFormat(cmp.Or(n.setting(ctx, numberFormatPath, scope), DefaultNumberFormat))
	if err != nil {
		log.Warn("order: the order number format is unusable, the default stands in", "err", err)
		f, _ = parseNumberFormat(DefaultNumberFormat)
	}
	count, period := "", ""
	if n.setting(ctx, numberCountPath, scope) == "channel" {
		if f.has("channel") {
			count = ch.ID.String()
		} else {
			log.Warn("order: a count per channel needs {channel} in the format, so the store's count runs", "channel", ch.Slug)
		}
	}
	if n.setting(ctx, numberResetPath, scope) == "yearly" {
		if f.has("yyyy", "yy") {
			period = strconv.Itoa(at.Year())
		} else {
			log.Warn("order: a yearly restart needs {yyyy} or {yy} in the format, so the count runs on", "channel", ch.Slug)
		}
	}
	seq, err := n.next(ctx, count, period)
	if err != nil {
		return "", fmt.Errorf("next order number: %w", err)
	}
	return f.render(seq, at, ch.Slug), nil
}
