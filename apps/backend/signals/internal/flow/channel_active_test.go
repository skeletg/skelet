package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	emailpkg "github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

type fixedStore struct{ active bool }

func (s fixedStore) Get(_ context.Context, id uuid.UUID) (Store, error) {
	return Store{ID: id, Name: "Shop", FromAddress: "shop@example.test", Active: s.active}, nil
}

type optedIn struct{}

func (optedIn) Get(_ context.Context, id uuid.UUID) (Contact, bool, error) {
	return Contact{ID: id, Email: "c@example.test", EmailOptedIn: true}, true, nil
}

type mailbox struct{ sent []emailpkg.Message }

func (m *mailbox) Send(_ context.Context, msg emailpkg.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

func mailingFlow(repo *fakeRepo, transactional bool) Flow {
	f := mkFlow(repo, []Node{{ID: "t", Type: NodeTrigger}, {ID: "m", Type: NodeSendEmail, Data: NodeData{Subject: "Hi"}}},
		[]Edge{{ID: "e", Source: "t", Target: "m"}})
	f.TriggerType = TriggerPasswordReset
	if transactional {
		f.TriggerConfig = json.RawMessage(`{"transactional":true}`)
	}
	repo.flows[f.ID] = f
	return f
}

func TestADeactivatedChannelSendsOnlyTransactionalMail(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name          string
		transactional bool
		want          int
	}{
		{"marketing", false, 0},
		{"transactional", true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := newFakeRepo()
			box := &mailbox{}
			svc := NewService(repo, emailtemplate.NewService(nil, nil), optedIn{}, fixedStore{active: false}, box, nil, nil, nil, false, emailtrack.NewSigner([]byte("k")))
			f := mailingFlow(repo, c.transactional)

			n, err := svc.EnrollMoment(ctx, f.TriggerType, f.ChannelID, uuid.New(), "", nil, "password_reset:"+uuid.NewString())
			if err != nil || n != c.want {
				t.Fatalf("enroll = %d, %v, want %d - a flow that won't send must not take the moment from core", n, err, c.want)
			}
			if len(box.sent) != c.want {
				t.Fatalf("mailed %d, want %d", len(box.sent), c.want)
			}

			box.sent = nil
			if err := svc.execEmail(ctx, f, f.Graph.Nodes[1], FlowRun{FlowID: f.ID, CustomerID: uuid.New()}); err != nil {
				t.Fatal(err)
			}
			if len(box.sent) != c.want {
				t.Fatalf("a run already under way mailed %d, want %d", len(box.sent), c.want)
			}
		})
	}
}
