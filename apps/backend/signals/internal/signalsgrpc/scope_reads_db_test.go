package signalsgrpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type contactsByID map[uuid.UUID]PreviewContact

func (c contactsByID) Get(_ context.Context, id uuid.UUID) (PreviewContact, bool, error) {
	p, ok := c[id]
	return p, ok, nil
}

func seedRosterCustomer(t *testing.T, qs *signalsdb.Queries, channelID uuid.UUID, email, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := qs.UpsertCustomerRosterEntry(context.Background(), signalsdb.UpsertCustomerRosterEntryParams{
		CustomerID: id, ChannelID: channelID, Email: email, FirstName: name, IsActive: true,
		Consents: json.RawMessage(`{}`), ReadAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed roster customer: %v", err)
	}
	return id
}

func TestAPreviewRendersOnlyACustomerOfItsOwnChannel(t *testing.T) {
	qs, deps := channelScopeServer(t)
	mine, theirs := uuid.New(), uuid.New()
	own := seedRosterCustomer(t, qs, mine, "own@example.test", "Owen")
	foreign := seedRosterCustomer(t, qs, theirs, "secret@example.test", "Sabine")
	deps.Roster = qs
	deps.Contacts = contactsByID{
		own:     {Email: "own@example.test", FirstName: "Owen"},
		foreign: {Email: "secret@example.test", FirstName: "Sabine"},
	}
	c := signalsv1.NewEmailBuilderServiceClient(dial(t, deps))
	ctx := tokenCtx(t, grantOn("emails.view", mine))
	preview := func(customer uuid.UUID) (*signalsv1.RenderLibraryPreviewResponse, error) {
		id := customer.String()
		return c.RenderLibraryPreview(ctx, &signalsv1.RenderLibraryPreviewRequest{
			ChannelId: mine.String(), Subject: "Hi {{customer.firstName}} {{customer.email}}", CustomerId: &id,
		})
	}

	res, err := preview(foreign)
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("previewing another channel's customer answered %s, want NotFound", got)
	}
	if strings.Contains(res.GetSubject()+res.GetHtml(), "Sabine") || strings.Contains(res.GetSubject()+res.GetHtml(), "secret@") {
		t.Errorf("the preview leaked the foreign customer: %q", res.GetSubject())
	}
	res, err = preview(own)
	if err != nil {
		t.Fatalf("previewing the channel's own customer: %v", err)
	}
	if !strings.Contains(res.GetSubject(), "Owen") {
		t.Errorf("subject = %q, want the channel's own customer merged in", res.GetSubject())
	}
}

func TestVariantStatsAreGatedOnTheCampaignsChannel(t *testing.T) {
	qs, deps := channelScopeServer(t)
	mine, theirs := uuid.New(), uuid.New()
	deps.Campaigns = campaign.NewService(campaign.NewRepository(qs), deps.Builder, nil, nil, nil, nil, nil, nil, nil, []byte("k"))
	camp, err := campaign.NewRepository(qs).Create(context.Background(), campaign.Campaign{
		ChannelID: theirs, Name: "Theirs", Subject: "s", AudienceKind: campaign.AudienceAll, Status: campaign.StatusDraft,
	})
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	c := signalsv1.NewPulseServiceClient(dial(t, deps))
	req := &signalsv1.GetCampaignVariantStatsRequest{CampaignId: camp.ID.String()}

	if _, err := c.GetCampaignVariantStats(tokenCtx(t, grantOn("campaigns.view", mine)), req); status.Code(err) != codes.PermissionDenied {
		t.Errorf("a grant on another channel answered %s, want PermissionDenied", status.Code(err))
	}
	if _, err := c.GetCampaignVariantStats(tokenCtx(t, grantOn("campaigns.view", theirs)), req); err != nil {
		t.Errorf("a grant on the campaign's own channel was refused: %v", err)
	}
	if _, err := c.GetCampaignVariantStats(systemCtx(t), req); err != nil {
		t.Errorf("the system caller was refused: %v", err)
	}
}
