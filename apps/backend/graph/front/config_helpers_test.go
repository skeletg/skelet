package front

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
)

// Front-API config reads resolve at the request's channel - reading them at the default scope is
// what made every channel share one store name, robots.txt and set of payment instructions
func TestRequestConfigScopeFollowsTheRequestChannel(t *testing.T) {
	id := uuid.New()
	withChannel := channel.WithChannel(context.Background(), &channel.Channel{ID: id})

	got := requestConfigScope(withChannel)
	if got.Kind != config.ScopeChannel || got.ChannelID == nil || *got.ChannelID != id {
		t.Errorf("requestConfigScope with a channel = %s, want that channel", config.FormatScope(got))
	}
	if got.Locale != nil {
		t.Errorf("scope carried locale %q - no knob is per-locale, and a locale would key the cache needlessly", *got.Locale)
	}

	// A probe or dev request with no channel resolved still has to answer something readable
	if bare := requestConfigScope(context.Background()); bare != config.DefaultScope {
		t.Errorf("requestConfigScope without a channel = %s, want the default scope", config.FormatScope(bare))
	}
}

// An order's instructions belong to the channel it was placed on, not to whichever storefront is
// rendering it
func TestChannelConfigScopeFromRecordID(t *testing.T) {
	id := uuid.New()
	got, ok := channelConfigScope(id.String())
	if !ok || got.ChannelID == nil || *got.ChannelID != id {
		t.Errorf("channelConfigScope(%q) = %s, ok=%v", id, config.FormatScope(got), ok)
	}

	// Callers fall back to the request scope on a miss, so this must report the miss rather than
	// silently pointing at the default scope
	if _, ok := channelConfigScope("not-a-uuid"); ok {
		t.Error("channelConfigScope accepted a non-uuid")
	}
	if _, ok := channelConfigScope(""); ok {
		t.Error("channelConfigScope accepted an empty id")
	}
}
