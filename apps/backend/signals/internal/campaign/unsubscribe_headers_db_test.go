package campaign

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

type publicStore struct{}

func (publicStore) Get(_ context.Context, id uuid.UUID) (Store, error) {
	return Store{ID: id, Name: "Shop", FromAddress: "shop@example.test", PublicURL: "https://shop.example.test", Active: true}, nil
}

func TestABroadcastCarriesOneClickUnsubscribeHeaders(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	channelID := uuid.New()
	seedDueCampaign(t, conn, channelID)
	sender := &capturingSender{}
	svc := &Service{
		repo: NewRepository(db.New(conn)), contacts: audienceOf(channelID, 2), tmpl: emailtemplate.NewService(emailtemplate.NewRepository(db.New(conn)), nil),
		stores: publicStore{}, sender: sender, recorder: &fakeRecorder{}, track: emailtrack.NewSigner([]byte("campaign-test-key")),
	}
	if _, err := svc.Drain(ctx, 5*time.Second); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("mailed %d, want 2", len(sender.sent))
	}
	for _, m := range sender.sent {
		if !strings.HasPrefix(m.Headers["List-Unsubscribe"], "<https://shop.example.test/unsubscribe/") {
			t.Errorf("mail to %s: List-Unsubscribe = %q", m.To, m.Headers["List-Unsubscribe"])
		}
		if m.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
			t.Errorf("mail to %s: List-Unsubscribe-Post = %q", m.To, m.Headers["List-Unsubscribe-Post"])
		}
	}
}
