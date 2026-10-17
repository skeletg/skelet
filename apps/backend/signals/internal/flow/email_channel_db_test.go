package flow

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

func TestAFlowSendsOnlyALibraryEmailOfItsOwnChannel(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	tmpl := emailtemplate.NewService(emailtemplate.NewRepository(db.New(conn)), nil)
	mine, theirs := uuid.New(), uuid.New()
	own, err := tmpl.CreateLibraryEmail(ctx, emailtemplate.LibraryEmail{ChannelID: mine, Name: "Own", Subject: "own"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := tmpl.CreateLibraryEmail(ctx, emailtemplate.LibraryEmail{ChannelID: theirs, Name: "Theirs", Subject: "theirs"})
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeRepo()
	box := &mailbox{}
	svc := NewService(repo, tmpl, optedIn{}, fixedStore{active: true}, box, nil, nil, nil, false, emailtrack.NewSigner([]byte("k")))
	graph := func(emailID uuid.UUID) Graph {
		return Graph{
			Nodes: []Node{{ID: "t", Type: NodeTrigger}, {ID: "m", Type: NodeSendEmail, Data: NodeData{EmailID: emailID.String()}}},
			Edges: []Edge{{ID: "e", Source: "t", Target: "m"}},
		}
	}
	in := func(emailID uuid.UUID) Input {
		return Input{ChannelID: mine, Name: "F", TriggerType: TriggerCustomerRegistered, Graph: graph(emailID)}
	}

	if _, err := svc.Create(ctx, in(foreign.ID)); err == nil {
		t.Error("a flow was saved sending another channel's email")
	}
	f, err := svc.Create(ctx, in(own.ID))
	if err != nil {
		t.Fatalf("own channel's email refused: %v", err)
	}
	if _, err := svc.Update(ctx, f.ID, in(foreign.ID)); err == nil {
		t.Error("a flow was updated to send another channel's email")
	}

	f.Graph = graph(foreign.ID)
	svc.execEmail(ctx, f, f.Graph.Nodes[1], FlowRun{FlowID: f.ID, CustomerID: uuid.New()})
	if len(box.sent) != 0 {
		t.Fatalf("mailed %q - another channel's email went out under this shop", box.sent[0].Subject)
	}
	f.Graph = graph(own.ID)
	svc.execEmail(ctx, f, f.Graph.Nodes[1], FlowRun{FlowID: f.ID, CustomerID: uuid.New()})
	if len(box.sent) != 1 {
		t.Fatalf("the flow's own email sent %d times, want 1", len(box.sent))
	}
}
