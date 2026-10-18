package signalsgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/behavioralsegment"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

// The spine as a behavioral refresh sees it - whoever fired the event, as the test sets them
type actorSpine struct {
	analytics.SpineStore
	actors []uuid.UUID
}

func (s *actorSpine) EventActors(context.Context, uuid.UUID, string, int, int, time.Time) ([]uuid.UUID, error) {
	return s.actors, nil
}

type enrollLog struct {
	flowEngine
	keys []string
}

func (e *enrollLog) Enroll(_ context.Context, _ string, _, customerID uuid.UUID, _ string, _ json.RawMessage, key string) int {
	e.keys = append(e.keys, key)
	return 1
}

// A segment repository whose member read fails, as a database blip would make it
type blindMembers struct{ segment.Repository }

func (blindMembers) MemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, errors.New("connection reset")
}

func enteredHarness(t *testing.T, repo func(segment.Repository) segment.Repository) (*maintenanceServer, *actorSpine, *enrollLog, uuid.UUID) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	base := segment.NewRepository(signalsdb.New(conn))
	segs := segment.NewService(repo(base), txrunner.New(conn), conn, nil)
	spine := &actorSpine{}
	behavioral := behavioralsegment.New(analytics.NewRecorder(signalsdb.New(conn), spine), segs)
	flows := &enrollLog{}
	ch := uuid.New()
	seg, err := behavioral.Save(context.Background(), behavioralsegment.SaveInput{
		ChannelID: ch.String(), Name: "Buyers " + uuid.NewString()[:6],
		Definition: behavioralsegment.Definition{Conditions: []behavioralsegment.Condition{{Event: "order_placed", Count: 1, Days: 30}}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Delete(context.Background(), seg.ID) })
	return &maintenanceServer{behavioral: behavioral, flows: flows}, spine, flows, seg.ID
}

func refreshOnce(t *testing.T, srv *maintenanceServer) error {
	t.Helper()
	_, err := srv.RefreshBehavioralSegments(auth.WithSystemCaller(context.Background()), &signalsv1.RefreshBehavioralSegmentsRequest{})
	return err
}

// Only a customer newly in the segment starts its flows, under a key a retried sweep reproduces
func TestSegmentEntriesEnrollOnceUnderAStableKey(t *testing.T) {
	srv, spine, flows, segID := enteredHarness(t, func(r segment.Repository) segment.Repository { return r })
	first, second := uuid.New(), uuid.New()
	spine.actors = []uuid.UUID{first}
	if err := refreshOnce(t, srv); err != nil {
		t.Fatal(err)
	}
	spine.actors = []uuid.UUID{first, second}
	if err := refreshOnce(t, srv); err != nil {
		t.Fatal(err)
	}
	var mine []string
	for _, k := range flows.keys {
		if strings.Contains(k, segID.String()) {
			mine = append(mine, k)
		}
	}
	day := time.Now().UTC().Format(time.DateOnly)
	want := []string{
		"segment_entered:" + segID.String() + ":" + first.String() + ":" + day,
		"segment_entered:" + segID.String() + ":" + second.String() + ":" + day,
	}
	if len(mine) != 2 || mine[0] != want[0] || mine[1] != want[1] {
		t.Fatalf("enrollments %v - want each newcomer once, keyed %v", mine, want)
	}
}

// A prior membership that can't be read makes everyone look new - the segment still refreshes, but starts no flows
func TestAnUnreadablePriorMembershipStartsNoFlows(t *testing.T) {
	srv, spine, flows, segID := enteredHarness(t, func(r segment.Repository) segment.Repository { return blindMembers{r} })
	spine.actors = []uuid.UUID{uuid.New(), uuid.New()}
	err := refreshOnce(t, srv)
	if err == nil {
		t.Error("a sweep that couldn't read a segment's members reported success")
	}
	for _, k := range flows.keys {
		if strings.Contains(k, segID.String()) {
			t.Fatalf("enrolled %s while the prior membership was unknown", k)
		}
	}
}
