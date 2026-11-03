package agenttools // the overview the chat opens with - the matching tools return the same sections, so the two read alike

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

const SnapshotMaxChars = 12000

type digest struct {
	title, what, empty string
	write              func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int, b *strings.Builder) error
}

var (
	salesDigest   = digest{"Sales", "sales", "No sales data in this window.", writeSales}
	lensDigest    = digest{"Lens (web traffic)", "web traffic", "No web traffic in this window.", writeLens}
	synapseDigest = digest{"Synapse (customer behaviour)", "customer behaviour", "No customer-behaviour data in this window.", writeSynapse}
	pulseDigest   = digest{"Pulse (email engagement)", "email engagement", "No email engagement in this window.", writePulse}
	prismDigest   = digest{"Prism (A/B experiments)", "experiments", "No experiments in this window.", writePrism}
)

func (d digest) read(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int) (string, error) {
	var b strings.Builder
	if err := d.write(ctx, deps, cid, since, days, &b); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

func (d digest) tool(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, days int) string {
	text, err := d.read(ctx, deps, cid, since, days)
	switch {
	case err != nil:
		return readFailed(ctx, err, d.what)
	case text == "":
		return d.empty
	}
	return "## " + d.title + "\n" + text
}

// A section that can't be read says so, so the model never takes an outage for a quiet week
func (s ToolSet) Snapshot(ctx context.Context, maxChars int) string {
	ctx = channel.WithChannel(ctx, s.ch)
	since := time.Now().AddDate(0, 0, -s.days)
	var b strings.Builder
	for _, sec := range []struct {
		on bool
		d  digest
	}{{s.perms.Sales, salesDigest}, {s.perms.Lens, lensDigest}, {s.perms.Synapse, synapseDigest}, {s.perms.Pulse, pulseDigest}, {s.perms.Prism, prismDigest}} {
		if !sec.on {
			continue
		}
		text, err := sec.d.read(ctx, s.deps, s.ch.ID, since, s.days)
		switch {
		case err != nil:
			slog.WarnContext(ctx, "snapshot: section failed", "section", sec.d.what, "err", err)
			fmt.Fprintf(&b, "## %s\n- could not be read right now, which is not the same as no data\n\n", sec.d.title)
		case text != "":
			fmt.Fprintf(&b, "## %s\n%s\n\n", sec.d.title, text)
		}
	}
	out := strings.TrimSpace(b.String())
	if maxChars <= 0 {
		maxChars = SnapshotMaxChars
	}
	if rs := []rune(out); len(rs) > maxChars {
		out = string(rs[:maxChars]) + "\n…(truncated - call a tool for the full detail)"
	}
	if out == "" {
		return "(no Signals data available for this channel and window)"
	}
	return out
}

// A section's main read decides whether it failed - a side read that fails just leaves its line out
func skipped(ctx context.Context, err error, what string) {
	slog.WarnContext(ctx, "tool: side read failed", "part", what, "err", err)
}

func joinN(count, limit int, sep string, item func(i int) string) string {
	parts := make([]string, min(count, limit))
	for i := range parts {
		parts[i] = item(i)
	}
	return strings.Join(parts, sep)
}

func writeSales(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, _ int, b *strings.Builder) error {
	f := sales.Filter{Start: since, End: time.Now(), ChannelIDs: []uuid.UUID{cid}}
	sum, err := deps.Sales.Summary(ctx, f)
	if err != nil || len(sum) == 0 {
		return err
	}
	for _, c := range sum {
		var aov int64
		if c.OrderCount > 0 {
			aov = c.GrossRevenue / int64(c.OrderCount)
		}
		fmt.Fprintf(b, "- %s: %d orders, gross %s, AOV %s, %d unique customers\n",
			c.CurrencyCode, c.OrderCount, money.FormatCode(c.GrossRevenue, c.CurrencyCode), money.FormatCode(aov, c.CurrencyCode), c.UniqueCustomers)
	}
	if tp, err := deps.Sales.TopProducts(ctx, f, 5); err != nil {
		skipped(ctx, err, "top products")
	} else if len(tp) > 0 {
		fmt.Fprintf(b, "Top products: %s\n", joinN(len(tp), 5, "; ", func(i int) string {
			name := tp[i].Name
			if name == "" {
				name = tp[i].SKU
			}
			return fmt.Sprintf("%s (%d units)", name, tp[i].UnitsSold)
		}))
	}
	if st, err := deps.Sales.ByStatus(ctx, f); err != nil {
		skipped(ctx, err, "orders by status")
	} else if len(st) > 0 {
		fmt.Fprintf(b, "Orders by status: %s\n", joinN(len(st), len(st), ", ", func(i int) string { return fmt.Sprintf("%s=%d", st[i].Status, st[i].OrderCount) }))
	}
	return nil
}

func writeLens(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, b *strings.Builder) error {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return err
	}
	ov, err := deps.Signals.Lens.GetOverview(authed, &signalsv1.GetOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "- %d page views, %d unique visitors\n", ov.GetViews(), ov.GetVisitors())
	if pages := ov.GetTopPages(); len(pages) > 0 {
		fmt.Fprintf(b, "Top pages: %s\n", joinN(len(pages), 5, "; ", func(i int) string {
			return fmt.Sprintf("%s (%d views)", untrusted(pages[i].GetPageKey()), pages[i].GetViews())
		}))
	}
	if devs := ov.GetDevices(); len(devs) > 0 {
		fmt.Fprintf(b, "Devices: %s\n", joinN(len(devs), len(devs), ", ", func(i int) string { return fmt.Sprintf("%s=%d", devs[i].GetLabel(), devs[i].GetViews()) }))
	}
	if refs := ov.GetReferrers(); len(refs) > 0 {
		fmt.Fprintf(b, "Top referrers: %s\n", joinN(len(refs), 5, "; ", func(i int) string {
			return fmt.Sprintf("%s (%d)", untrusted(refs[i].GetLabel()), refs[i].GetViews())
		}))
	}
	if vs, err := deps.Signals.Lens.GetVitalsSummary(authed, &signalsv1.GetVitalsSummaryRequest{ChannelId: cid.String(), SinceDays: int32(days)}); err != nil {
		skipped(ctx, err, "web vitals")
	} else if v := vs.GetVitals(); len(v) > 0 {
		fmt.Fprintf(b, "Web vitals: %s\n", joinN(len(v), len(v), ", ", func(i int) string { return fmt.Sprintf("%s p75=%.0f", v[i].GetMetric(), v[i].GetP75()) }))
	}
	return nil
}

func writeSynapse(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, b *strings.Builder) error {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return err
	}
	ov, err := deps.Signals.Synapse.GetSynapseOverview(authed, &signalsv1.GetSynapseOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "- %d events, %d page views, %d conversions, %d identified customers, %d anonymous visitors\n",
		ov.GetTotalEvents(), ov.GetPageViews(), ov.GetConversions(), ov.GetIdentifiedCustomers(), ov.GetAnonymousVisitors())
	for _, rev := range ov.GetRevenue() {
		fmt.Fprintf(b, "Tracked revenue: %s across %d orders\n", money.FormatCode(rev.GetCents(), rev.GetCurrency()), rev.GetOrders())
	}
	if evs := ov.GetTopEvents(); len(evs) > 0 {
		fmt.Fprintf(b, "Top events: %s\n", joinN(len(evs), 8, ", ", func(i int) string { return fmt.Sprintf("%s=%d", untrusted(evs[i].GetName()), evs[i].GetCount()) }))
	}
	if segs, err := deps.Signals.Segments.ListSegments(authed, &signalsv1.ListSegmentsRequest{PageSize: 50}); err != nil {
		skipped(ctx, err, "segments")
	} else if sg := segs.GetSegments(); len(sg) > 0 {
		fmt.Fprintf(b, "Segments: %s\n", joinN(len(sg), len(sg), "; ", func(i int) string { return fmt.Sprintf("%s (%d members)", sg[i].GetName(), sg[i].GetMemberCount()) }))
	}
	return nil
}

func writePulse(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, b *strings.Builder) error {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return err
	}
	e, err := deps.Signals.Pulse.GetEngagement(authed, &signalsv1.GetEngagementRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "- %d opens (human), %d clicks, %d conversions\n", e.GetOpensHuman(), e.GetClicks(), e.GetConversions())
	for _, rev := range e.GetRevenue() {
		fmt.Fprintf(b, "Attributed revenue: %s across %d orders\n", money.FormatCode(rev.GetCents(), rev.GetCurrency()), rev.GetOrders())
	}
	if links := e.GetTopLinks(); len(links) > 0 {
		fmt.Fprintf(b, "Top links: %s\n", joinN(len(links), 5, "; ", func(i int) string { return fmt.Sprintf("%s (%d clicks)", links[i].GetUrl(), links[i].GetClicks()) }))
	}
	return nil
}

func writePrism(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, days int, b *strings.Builder) error {
	authed, err := signalsCall(ctx, deps)
	if err != nil {
		return err
	}
	ov, err := deps.Signals.Prism.GetExperimentsOverview(authed, &signalsv1.GetExperimentsOverviewRequest{ChannelId: cid.String(), SinceDays: int32(days)})
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "- %d running, %d with a decided winner, %d total exposed\n", ov.GetRunning(), ov.GetDecided(), ov.GetTotalExposed())
	for _, s := range ov.GetExperiments() {
		winner := ""
		if s.GetWinner() != "" {
			winner = fmt.Sprintf(", winner: %s", s.GetWinner())
		}
		fmt.Fprintf(b, "- %s (id %s): leader %q (uplift %.1f%%, chance-to-win %.0f%%)%s\n",
			s.GetName(), s.GetId(), s.GetLeader(), s.GetLeaderUplift()*100, s.GetLeaderChanceToWin()*100, winner)
	}
	return nil
}
