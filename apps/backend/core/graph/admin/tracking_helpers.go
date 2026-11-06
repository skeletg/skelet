package admin

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	trackingView   = "tracking.view"
	trackingManage = "tracking.manage"
)

// requireTrackingChannel gates on the channel the operation names
func (r *Resolver) requireTrackingChannel(ctx context.Context, slug, channelID string) (uuid.UUID, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return uuid.Nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return uuid.Nil, errInvalidID("channelId", err)
	}
	return cid, auth.RequireChannelPermission(ctx, r.AdminSvc, slug, cid, nil)
}

// requireTrackingDestination loads the destination, then gates on its own channel - an id from another channel is refused
func (r *Resolver) requireTrackingDestination(ctx context.Context, slug, id string) (tracking.Destination, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return tracking.Destination{}, err
	}
	did, err := uuid.Parse(id)
	if err != nil {
		return tracking.Destination{}, errInvalidID("id", err)
	}
	d, err := r.TrackingSvc.Get(ctx, did)
	if errors.Is(err, sql.ErrNoRows) {
		return tracking.Destination{}, errNotFound("tracking destination")
	}
	if err != nil {
		return tracking.Destination{}, err
	}
	return d, auth.RequireChannelPermission(ctx, r.AdminSvc, slug, d.ChannelID, nil)
}

// requireTrackingDelivery follows a delivery to its destination's channel
func (r *Resolver) requireTrackingDelivery(ctx context.Context, slug, id string) (tracking.Delivery, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return tracking.Delivery{}, err
	}
	did, err := uuid.Parse(id)
	if err != nil {
		return tracking.Delivery{}, errInvalidID("id", err)
	}
	d, err := r.TrackingSvc.Delivery(ctx, did)
	if err != nil {
		return tracking.Delivery{}, errNotFound("tracking delivery")
	}
	if _, err := r.requireTrackingDestination(ctx, slug, d.DestinationID.String()); err != nil {
		return tracking.Delivery{}, err
	}
	return d, nil
}

// The destination with its current health
func (r *Resolver) trackingDestinationWithHealth(ctx context.Context, d tracking.Destination) (*model.TrackingDestination, error) {
	health, err := r.TrackingSvc.Health(ctx, d.ChannelID)
	if err != nil {
		return nil, err
	}
	return toTrackingDestination(d, health[d.ID]), nil
}

// The secret field's key for a kind - its value travels as Destination.Secret, never in the settings list
func secretKey(k tracking.Kind) string {
	for _, f := range k.Fields {
		if f.Secret {
			return f.Key
		}
	}
	return ""
}

// destinationFromInput takes only the kind's own settings, and moves the secret field's value to Secret
func destinationFromInput(k tracking.Kind, in model.TrackingDestinationInput) tracking.Destination {
	d := tracking.Destination{Kind: k.Kind, Name: strings.TrimSpace(in.Name), Events: in.Events, Config: map[string]string{}}
	if in.Enabled != nil {
		d.Enabled = *in.Enabled
	}
	secret := secretKey(k)
	for _, c := range in.Config {
		switch {
		case c == nil:
		case c.Key == secret && secret != "":
			d.Secret = c.Value
		case slices.ContainsFunc(k.Fields, func(f sdk.DestinationField) bool { return f.Key == c.Key }):
			if v := strings.TrimSpace(c.Value); v != "" {
				d.Config[c.Key] = v
			}
		}
	}
	return d
}

func toTrackingDestination(d tracking.Destination, h tracking.Health) *model.TrackingDestination {
	k, _ := tracking.KindFor(d.Kind)
	m := &model.TrackingDestination{
		ID: d.ID.String(), ChannelID: d.ChannelID.String(), Kind: d.Kind, Name: d.Name, Enabled: d.Enabled,
		Events: nonNilStrings(d.Events), EffectiveEvents: nonNilStrings(d.Events),
		Config: configValues(d.Config, secretKey(k)), SecretSet: d.Secret != "", NeedsSecret: tracking.NeedsSecret(d),
		InTestMode: tracking.InTestMode(d),
		Health:     &model.TrackingHealth{LastSentAt: d.LastSentAt, LastError: h.LastError, Dead24h: int32(h.Dead24h)},
		CreatedAt:  d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if len(d.Events) == 0 {
		m.EffectiveEvents = nonNilStrings(k.DefaultEvents)
	}
	if d.ImportedFrom != nil {
		from := d.ImportedFrom.String()
		m.ImportedFrom = &from
	}
	return m
}

// The settings in key order, the secret field left out
func configValues(cfg map[string]string, secret string) []*model.TrackingConfigValue {
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		if k != secret {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]*model.TrackingConfigValue, 0, len(keys))
	for _, k := range keys {
		out = append(out, &model.TrackingConfigValue{Key: k, Value: cfg[k]})
	}
	return out
}

func toTrackingDelivery(d tracking.Delivery) *model.TrackingDelivery {
	return &model.TrackingDelivery{
		ID: d.ID.String(), EventName: d.EventName, EventID: d.EventID, Status: model.TrackingDeliveryStatus(strings.ToUpper(d.Status)), Attempts: d.Attempts,
		LastError: d.LastError, NextAttemptAt: d.NextAttemptAt, CreatedAt: d.CreatedAt,
	}
}

// Every first-party platform, plus any third-party kind this build registers, as installed or not
func (r *Resolver) trackingPlatforms(ctx context.Context, channelID uuid.UUID) []*model.TrackingPlatform {
	var out []*model.TrackingPlatform
	seen := map[string]bool{}
	for _, k := range tracking.Kinds() {
		seen[k.Kind] = true
		p := &model.TrackingPlatform{
			Kind: k.Kind, Label: k.Label, PluginSlug: k.Plugin, Installed: true,
			AvailableOnChannel: k.Available == nil || k.Available(ctx, channelID),
			DefaultEvents:      nonNilStrings(k.DefaultEvents), Purposes: []string{}, Fields: []*model.TrackingField{},
		}
		for _, purpose := range k.Purposes {
			p.Purposes = append(p.Purposes, string(purpose))
		}
		for _, f := range k.Fields {
			p.Fields = append(p.Fields, &model.TrackingField{Key: f.Key, Label: f.Label, Help: f.Help, Required: f.Required, Secret: f.Secret, TestMode: f.TestMode})
		}
		if k.NecessarySetting != "" {
			setting := k.NecessarySetting
			p.NecessarySetting = &setting
		}
		out = append(out, p)
	}
	for _, e := range tracking.FirstParty {
		if !seen[e.Kind] {
			out = append(out, &model.TrackingPlatform{
				Kind: e.Kind, Label: e.Label, PluginSlug: e.Plugin,
				Purposes: []string{}, DefaultEvents: []string{}, Fields: []*model.TrackingField{},
			})
		}
	}
	// The catalogue's own order, so the list reads the same in every build; a third-party kind goes last
	rank := func(kind string) int {
		if i := slices.IndexFunc(tracking.FirstParty, func(e tracking.CatalogEntry) bool { return e.Kind == kind }); i >= 0 {
			return i
		}
		return len(tracking.FirstParty)
	}
	slices.SortStableFunc(out, func(a, b *model.TrackingPlatform) int { return rank(a.Kind) - rank(b.Kind) })
	return out
}

// importCandidate is what one Signals destination would become here - or why it can't
func importCandidate(d *signalsv1.EventDestination, existing map[uuid.UUID]uuid.UUID) *model.SignalsImportCandidate {
	c := &model.SignalsImportCandidate{
		LegacyID: d.GetId(), Name: d.GetName(), LegacyKind: d.GetKind(), SignalsActive: d.GetIsActive(),
		Config: []*model.TrackingConfigValue{}, DroppedFilters: []string{},
	}
	for _, s := range d.GetEventSources() {
		c.DroppedFilters = append(c.DroppedFilters, "source: "+s)
	}
	for _, n := range d.GetEventNames() {
		c.DroppedFilters = append(c.DroppedFilters, "event: "+n)
	}
	if legacy, err := uuid.Parse(d.GetId()); err == nil {
		if as, ok := existing[legacy]; ok {
			s := as.String()
			c.ImportedAs = &s
		}
	}
	kind, cfg, err := legacyTarget(d)
	if err != nil {
		problem := err.Error()
		c.Problem = &problem
	}
	if kind != "" { // mapped, even when its plugin is missing
		c.TargetKind = &kind
		c.Config = configValues(cfg, "")
	}
	return c
}

// legacyTarget is the kind and settings a Signals destination becomes here - refused when it doesn't map, or maps to a
// plugin this build lacks. The preview and the import both go through it, so what one offers the other accepts
func legacyTarget(d *signalsv1.EventDestination) (string, map[string]string, error) {
	kind, cfg, err := tracking.FromLegacy(d.GetKind(), d.GetTargetUrl())
	if err != nil {
		return "", nil, err
	}
	if _, ok := tracking.KindFor(kind); !ok {
		return kind, cfg, apperr.New("TRACKING_PLUGIN_MISSING", apperr.ClassValidation, "the "+kind+" plugin isn't in this build")
	}
	return kind, cfg, nil
}

// The channel's Signals event destinations - none when Signals is off. Reading them needs settings.manage there too
func (r *Resolver) signalsDestinations(ctx context.Context, channelID string) ([]*signalsv1.EventDestination, error) {
	authed, err := r.signalsChannel(ctx, "settings.manage", channelID)
	if err != nil || !r.SignalsEnabled {
		return nil, err
	}
	res, err := r.SignalsAPI.Destination.ListDestinations(authed, &signalsv1.ListDestinationsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return res.GetDestinations(), nil
}

// The channel's destinations by the Signals one each was imported from
func importedFrom(ds []tracking.Destination) map[uuid.UUID]uuid.UUID {
	out := map[uuid.UUID]uuid.UUID{}
	for _, d := range ds {
		if d.ImportedFrom != nil {
			out[*d.ImportedFrom] = d.ID
		}
	}
	return out
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
