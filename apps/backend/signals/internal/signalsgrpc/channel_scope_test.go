package signalsgrpc

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

func grantOn(slug string, channelID uuid.UUID) auth.Grant {
	return auth.Grant{Slug: slug, ChannelID: &channelID}
}

func TestAChannelGrantCannotNameAnotherChannel(t *testing.T) {
	conn := testConn(t)
	prism, dest, pulse := signalsv1.NewPrismServiceClient(conn), signalsv1.NewDestinationServiceClient(conn), signalsv1.NewPulseServiceClient(conn)
	mine, theirs := uuid.New(), uuid.New()
	ctx := tokenCtx(t,
		grantOn("experiments.manage", mine), grantOn("settings.manage", mine), grantOn("emails.view", mine))
	other := theirs.String()

	for name, call := range map[string]func() error{
		"ListExperiments": func() error {
			_, e := prism.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: other})
			return e
		},
		"GetExperimentsOverview": func() error {
			_, e := prism.GetExperimentsOverview(ctx, &signalsv1.GetExperimentsOverviewRequest{ChannelId: other})
			return e
		},
		"ListGoalEvents": func() error {
			_, e := prism.ListGoalEvents(ctx, &signalsv1.ListGoalEventsRequest{ChannelId: other})
			return e
		},
		"CreateExperiment": func() error {
			_, e := prism.CreateExperiment(ctx, &signalsv1.CreateExperimentRequest{Experiment: &signalsv1.ExperimentInput{ChannelId: other, Key: "k", Name: "n"}})
			return e
		},
		"ListDestinations": func() error {
			_, e := dest.ListDestinations(ctx, &signalsv1.ListDestinationsRequest{ChannelId: other})
			return e
		},
		"CreateDestination": func() error {
			_, e := dest.CreateDestination(ctx, &signalsv1.CreateDestinationRequest{Destination: &signalsv1.DestinationInput{ChannelId: other, Name: "n"}})
			return e
		},
		"GetEngagement": func() error {
			_, e := pulse.GetEngagement(ctx, &signalsv1.GetEngagementRequest{ChannelId: other})
			return e
		},
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s on another channel answered %s, want PermissionDenied", name, got)
		}
	}
	if got := status.Code(mustErr(prism.ListExperiments(ctx, &signalsv1.ListExperimentsRequest{ChannelId: mine.String()}))); got == codes.PermissionDenied {
		t.Error("a grant was refused on its own channel")
	}
}

type allowAllGuard struct{}

func (allowAllGuard) ValidateTargetURL(string, bool) error { return nil }
func (allowAllGuard) SafeTransport(bool) *http.Transport {
	return http.DefaultTransport.(*http.Transport)
}

func channelScopeServer(t *testing.T) (*signalsdb.Queries, Deps) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	qs := signalsdb.New(conn)
	return qs, Deps{
		ServiceKeys: testKeys,
		Channels:    analytics.NewChannelRoster(nil),
		Experiment:  experiment.NewService(experiment.NewRepository(qs, nil)),
		Destination: destination.NewService(qs, nil, allowAllGuard{}, nil, true),
		Lens:        lens.NewService(qs, nil, nil, nil),
		Builder:     emailtemplate.NewService(emailtemplate.NewRepository(qs), nil),
	}
}

func TestAnExperimentByIdIsGatedOnItsOwnChannel(t *testing.T) {
	qs, deps := channelScopeServer(t)
	c := signalsv1.NewPrismServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	exp, err := deps.Experiment.Create(context.Background(), experiment.Input{
		ChannelID: theirs, Key: "hero", Name: "Hero", Status: "draft", Coverage: 100,
		Variations: []experiment.Variation{{Key: "a", Weight: 50}, {Key: "b", Weight: 50}},
	})
	if err != nil {
		t.Fatalf("seed experiment: %v", err)
	}
	id := exp.ID.String()
	ctx := tokenCtx(t, grantOn("experiments.manage", mine))

	for name, call := range map[string]func() error{
		"GetExperiment": func() error { _, e := c.GetExperiment(ctx, &signalsv1.GetExperimentRequest{Id: id}); return e },
		"GetExperimentResults": func() error {
			_, e := c.GetExperimentResults(ctx, &signalsv1.GetExperimentResultsRequest{Id: id})
			return e
		},
		"UpdateExperiment": func() error {
			_, e := c.UpdateExperiment(ctx, &signalsv1.UpdateExperimentRequest{Id: id, Experiment: &signalsv1.ExperimentInput{
				ChannelId: mine.String(), Key: "hero", Name: "Hijacked", Status: "running", Coverage: 100,
				Variations: []*signalsv1.ExperimentVariation{{Key: "a", Weight: 50}, {Key: "b", Weight: 50}},
			}})
			return e
		},
		"DeleteExperiment": func() error { _, e := c.DeleteExperiment(ctx, &signalsv1.DeleteExperimentRequest{Id: id}); return e },
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s on another channel's experiment answered %s, want PermissionDenied", name, got)
		}
	}
	row, err := qs.GetExperiment(context.Background(), exp.ID)
	if err != nil {
		t.Fatalf("the experiment is gone: %v", err)
	}
	if row.Name != "Hero" || row.Status != "draft" {
		t.Errorf("the experiment was written through a foreign grant: name %q, status %q", row.Name, row.Status)
	}
	if _, err := c.GetExperiment(tokenCtx(t, grantOn("experiments.view", theirs)), &signalsv1.GetExperimentRequest{Id: id}); err != nil {
		t.Errorf("a grant on the experiment's own channel was refused: %v", err)
	}
}

func TestADestinationByIdIsGatedOnItsOwnChannel(t *testing.T) {
	qs, deps := channelScopeServer(t)
	c := signalsv1.NewDestinationServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	row, err := qs.CreateEventDestination(context.Background(), signalsdb.CreateEventDestinationParams{
		ChannelID: theirs, Name: "theirs", Kind: destination.KindWebhook, TargetUrl: "https://theirs.example/hook",
		EventSources: []string{}, EventNames: []string{}, IsActive: true,
	})
	if err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	id := row.ID.String()
	ctx := tokenCtx(t, grantOn("settings.manage", mine))

	for name, call := range map[string]func() error{
		"GetDestination": func() error {
			_, e := c.GetDestination(ctx, &signalsv1.GetDestinationRequest{Id: id})
			return e
		},
		"UpdateDestination": func() error {
			_, e := c.UpdateDestination(ctx, &signalsv1.UpdateDestinationRequest{Id: id, Destination: &signalsv1.DestinationInput{
				ChannelId: mine.String(), Name: "mine", Kind: destination.KindWebhook, TargetUrl: "https://attacker.example/hook",
			}})
			return e
		},
		"DeleteDestination": func() error {
			_, e := c.DeleteDestination(ctx, &signalsv1.DeleteDestinationRequest{Id: id})
			return e
		},
		"SyncAudience": func() error {
			_, e := c.SyncAudience(ctx, &signalsv1.SyncAudienceRequest{SegmentId: uuid.New().String(), DestinationId: id})
			return e
		},
	} {
		if got := status.Code(call()); got != codes.PermissionDenied {
			t.Errorf("%s on another channel's destination answered %s, want PermissionDenied", name, got)
		}
	}
	after, err := qs.GetEventDestination(context.Background(), row.ID)
	if err != nil {
		t.Fatalf("the destination is gone: %v", err)
	}
	if after.TargetUrl != "https://theirs.example/hook" {
		t.Errorf("the target was repointed through a foreign grant: %q", after.TargetUrl)
	}
	if _, err := c.GetDestination(tokenCtx(t, grantOn("settings.manage", theirs)), &signalsv1.GetDestinationRequest{Id: id}); err != nil {
		t.Errorf("a grant on the destination's own channel was refused: %v", err)
	}
}

func TestARevisionIsGatedBeforeItIsRestored(t *testing.T) {
	_, deps := channelScopeServer(t)
	c := signalsv1.NewEmailBuilderServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	bg := context.Background()
	e, err := deps.Builder.CreateLibraryEmail(bg, emailtemplate.LibraryEmail{ChannelID: theirs, Name: "Welcome", Subject: "old"})
	if err != nil {
		t.Fatalf("seed email: %v", err)
	}
	if err := deps.Builder.Snapshot(bg, e, nil); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	revs, err := deps.Builder.ListRevisions(bg, e.ID, 1)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions: %v (%d)", err, len(revs))
	}
	e.Subject = "current"
	if _, err := deps.Builder.UpdateLibraryEmail(bg, e); err != nil {
		t.Fatalf("update email: %v", err)
	}

	_, err = c.RestoreEmailRevision(tokenCtx(t, grantOn("emails.manage", mine)), &signalsv1.RestoreEmailRevisionRequest{RevisionId: revs[0].ID.String()})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Errorf("restoring another channel's revision answered %s, want PermissionDenied", got)
	}
	got, _, err := deps.Builder.GetLibraryEmail(bg, e.ID)
	if err != nil {
		t.Fatalf("reload email: %v", err)
	}
	if got.Subject != "current" {
		t.Errorf("a refused restore still wrote the email: subject %q", got.Subject)
	}
}

func TestVisitorSessionsKeepToTheCallersChannels(t *testing.T) {
	qs, deps := channelScopeServer(t)
	c := signalsv1.NewLensServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	anon := "visitor-" + uuid.NewString()[:8]
	now := time.Now()
	ownSession, foreignSession := uuid.New(), uuid.New()
	for _, s := range []struct{ id, channel uuid.UUID }{{ownSession, mine}, {foreignSession, theirs}} {
		if _, err := qs.LensUpsertSession(context.Background(), signalsdb.LensUpsertSessionParams{
			ID: s.id, ChannelID: s.channel, AnonID: anon, StartedAt: now, LastEventAt: now, EventCount: 1,
		}); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}

	res, err := c.ListVisitorSessions(tokenCtx(t, grantOn("lens.view", mine)), &signalsv1.ListVisitorSessionsRequest{AnonId: anon})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.GetSessions()) != 1 || res.GetSessions()[0].GetId() != ownSession.String() {
		t.Errorf("sessions = %+v, want only the one on the caller's channel", res.GetSessions())
	}
	all, err := c.ListVisitorSessions(tokenCtx(t), &signalsv1.ListVisitorSessionsRequest{AnonId: anon})
	if err != nil {
		t.Fatalf("list as superuser: %v", err)
	}
	if len(all.GetSessions()) != 2 {
		t.Errorf("a superuser saw %d sessions, want both", len(all.GetSessions()))
	}
}

func TestAReplayFlushCannotJoinAForeignSession(t *testing.T) {
	qs, _ := channelScopeServer(t)
	bg := context.Background()
	id, channel, now := uuid.New(), uuid.New(), time.Now()
	seed := signalsdb.LensUpsertSessionParams{ID: id, ChannelID: channel, AnonID: "owner", StartedAt: now, LastEventAt: now, EventCount: 1}
	if n, err := qs.LensUpsertSession(bg, seed); err != nil || n != 1 {
		t.Fatalf("seed session: n=%d err=%v", n, err)
	}
	otherChannel, otherVisitor := seed, seed
	otherChannel.ChannelID = uuid.New()
	otherVisitor.AnonID = "intruder"
	for name, p := range map[string]signalsdb.LensUpsertSessionParams{"another channel": otherChannel, "another visitor": otherVisitor} {
		if n, err := qs.LensUpsertSession(bg, p); err != nil || n != 0 {
			t.Errorf("a flush from %s touched the session: n=%d err=%v", name, n, err)
		}
	}
	if n, err := qs.LensUpsertSession(bg, seed); err != nil || n != 1 {
		t.Errorf("the owner's own next flush was refused: n=%d err=%v", n, err)
	}
}
