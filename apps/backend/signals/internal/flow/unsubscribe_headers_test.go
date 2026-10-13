package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

type publicStore struct{}

func (publicStore) Get(_ context.Context, id uuid.UUID) (Store, error) {
	return Store{ID: id, Name: "Shop", FromAddress: "shop@example.test", PublicURL: "https://shop.example.test", Active: true}, nil
}

func TestOnlyMarketingFlowMailCarriesListUnsubscribe(t *testing.T) {
	for _, c := range []struct {
		name          string
		transactional bool
		want          bool
	}{
		{"marketing", false, true},
		{"transactional", true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := newFakeRepo()
			box := &mailbox{}
			svc := NewService(repo, emailtemplate.NewService(nil, nil), optedIn{}, publicStore{}, box, nil, nil, nil, false, []byte("k"))
			f := mailingFlow(repo, c.transactional)
			svc.execEmail(context.Background(), f, f.Graph.Nodes[1], FlowRun{FlowID: f.ID, CustomerID: uuid.New()})
			if len(box.sent) != 1 {
				t.Fatalf("mailed %d, want 1", len(box.sent))
			}
			h := box.sent[0].Headers
			got := strings.HasPrefix(h["List-Unsubscribe"], "<https://shop.example.test/unsubscribe/") && h["List-Unsubscribe-Post"] == "List-Unsubscribe=One-Click"
			if got != c.want {
				t.Errorf("one-click unsubscribe headers present = %v, want %v (headers %v)", got, c.want, h)
			}
		})
	}
}
