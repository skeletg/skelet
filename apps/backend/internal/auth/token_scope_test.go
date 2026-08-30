package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// A channel-pinned token must behave like an admin whose only grants are channel-scoped: unscoped
// checks pass, its own channel passes, global gates and other channels refuse
func TestChannelPinnedTokenScoping(t *testing.T) {
	mine, other := uuid.New(), uuid.New()
	perms := map[string]struct{}{"catalog.view": {}}
	pinned := &ApiTokenClaims{TokenID: uuid.New(), Name: "feed", Permissions: perms, ChannelID: &mine}
	global := &ApiTokenClaims{TokenID: uuid.New(), Name: "feed", Permissions: perms}

	if err := requireTokenPermission(pinned, "catalog.view"); err != nil {
		t.Errorf("unscoped check on pinned token: %v", err)
	}
	if err := requireChannelTokenPermission(pinned, "catalog.view", mine); err != nil {
		t.Errorf("own channel: %v", err)
	}
	if err := requireChannelTokenPermission(pinned, "catalog.view", other); err == nil {
		t.Error("other channel passed on a pinned token")
	}
	if err := requireGlobalTokenPermission(pinned, "catalog.view"); err == nil {
		t.Error("global gate passed on a pinned token")
	}
	if err := requireGlobalTokenPermission(global, "catalog.view"); err != nil {
		t.Errorf("global gate on unpinned token: %v", err)
	}
	if err := requireChannelTokenPermission(global, "catalog.view", other); err != nil {
		t.Errorf("any channel on unpinned token: %v", err)
	}
	// a held slug is still required everywhere
	if err := requireChannelTokenPermission(pinned, "orders.view", mine); err == nil {
		t.Error("unheld slug passed")
	}
}

func TestLoadPermittedChannelsForTokens(t *testing.T) {
	mine := uuid.New()
	ctx := WithApiToken(context.Background(), &ApiTokenClaims{TokenID: uuid.New(), ChannelID: &mine})
	p, err := LoadPermittedChannels(ctx, nil, "catalog.view")
	if err != nil {
		t.Fatal(err)
	}
	if p.Bypass() || !p.Allows(mine) || p.Allows(uuid.New()) {
		t.Errorf("pinned token sees %v, want exactly its channel", p)
	}

	ctx = WithApiToken(context.Background(), &ApiTokenClaims{TokenID: uuid.New()})
	p, err = LoadPermittedChannels(ctx, nil, "catalog.view")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Bypass() {
		t.Error("unpinned token lost the historical bypass")
	}
}
