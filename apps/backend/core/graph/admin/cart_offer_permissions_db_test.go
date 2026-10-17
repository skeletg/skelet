package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type credentialPreviewMailer struct{ calls int }

func (*credentialPreviewMailer) SendCartOffer(context.Context, OfferMail) error { return nil }
func (m *credentialPreviewMailer) PreviewCartOffer(_ context.Context, in OfferMail) (OfferMailPreview, error) {
	m.calls++
	return OfferMailPreview{HTML: "<a href=\"" + in.Link + "\">Open</a>", Text: in.Link}, nil
}

func TestOfferCredentialsRequireUpdateOnTheirChannel(t *testing.T) {
	conn := testdb.Require(t)
	q, tx := db.New(conn), txrunner.New(conn)
	var ch uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	svc := cart.NewService(cart.NewRepository(q), tx, nil)
	svc.SetOfferLinks(cart.NewOfferLinks(svc.Repo()))
	c, err := svc.AdminCreateOffer(context.Background(), cart.CreateOfferInput{ChannelID: ch, Email: "preview@example.test", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	mail := &credentialPreviewMailer{}
	r := &Resolver{CartSvc: svc, ChannelSvc: channel.NewService(channel.NewRepository(q, tx)), AdminSvc: adminpkg.NewService(adminpkg.NewRepository(q, tx)), OfferMail: mail}
	other := uuid.New()
	for _, tc := range []struct {
		name    string
		grants  []auth.Grant
		allowed bool
	}{
		{"view only", []auth.Grant{{Slug: "orders.view", ChannelID: &ch}}, false},
		{"update another channel", []auth.Grant{{Slug: "orders.view", ChannelID: &ch}, {Slug: "orders.update", ChannelID: &other}}, false},
		{"update this channel", []auth.Grant{{Slug: "orders.view", ChannelID: &ch}, {Slug: "orders.update", ChannelID: &ch}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := adminWithCachedGrants(tc.grants...)
			got := r.toAdminCartOfferModel(ctx, c)
			if tc.allowed {
				if !strings.Contains(got.LinkPath, c.Offer.LinkToken) {
					t.Fatalf("missing credential: %+v", got)
				}
			} else if got.Link != "" || got.LinkPath != "" {
				t.Fatalf("credential leaked: %+v", got)
			}
			before := mail.calls
			preview, err := (&queryResolver{r}).AdminCartOfferEmailPreview(ctx, c.ID.String())
			if tc.allowed {
				if err != nil || !strings.Contains(preview.HTML, c.Offer.LinkToken) || !strings.Contains(preview.Text, c.Offer.LinkToken) {
					t.Fatalf("preview=%+v, %v", preview, err)
				}
			} else if err == nil || preview != nil || mail.calls != before {
				t.Fatalf("preview credential exposed: %+v, %v", preview, err)
			}
		})
	}
	// A failed settings validation must not persist the checkout prepared by the resolver.
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	for _, bad := range []model.AdminUpdateCartOfferInput{
		{ExpiresAt: time.Now().Add(time.Hour), Message: strings.Repeat("x", 2001)},
		{ExpiresAt: time.Now().Add(time.Hour), ShowInAccount: true},
	} {
		bad.ShippingAddress = &model.AdminCartAddressInput{FirstName: "Before", LastName: "Failure", Street1: "1 Test St", City: "City", PostalCode: "12345", CountryCode: "US"}
		if _, err := (&mutationResolver{r}).AdminUpdateCartOffer(ctx, c.ID.String(), bad); err == nil {
			t.Fatal("invalid settings saved")
		}
		got, err := svc.GetCartByIDReadOnly(ctx, c.ID)
		if err != nil || got.Checkout.ShippingAddress != nil {
			t.Fatalf("partial checkout persisted: %+v, %v", got.Checkout, err)
		}
	}
}
