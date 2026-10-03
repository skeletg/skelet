package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestAChannelAdminCannotRerouteOrRemoveTheirChannel(t *testing.T) {
	conn := testdb.Require(t)
	svc := channel.NewService(channel.NewRepository(db.New(conn), txrunner.New(conn)))
	suffix := uuid.NewString()[:8]
	ch, err := svc.Create(context.Background(), channel.CreateChannelInput{Slug: "az-" + suffix, Name: "AZ", CurrencyCode: "USD", PublicURL: "https://az-" + suffix + ".test"})
	if err != nil {
		t.Fatal(err)
	}
	r := &mutationResolver{&Resolver{ChannelSvc: svc}}
	scoped := adminWithCachedGrants(auth.Grant{Slug: channel.PermManage, ChannelID: &ch.ID})
	global := adminWithCachedGrants(auth.Grant{Slug: channel.PermManage})
	id := ch.ID.String()

	name, same, moved := "Renamed", ch.PublicURL, "https://victim-"+suffix+".test"
	if _, err := r.UpdateChannel(scoped, model.UpdateChannelInput{ID: id, Name: &name, PublicURL: &same}); err != nil {
		t.Fatalf("a channel admin renaming their channel and resending its URL: %v", err)
	}
	if _, err := r.UpdateChannel(scoped, model.UpdateChannelInput{ID: id, PublicURL: &moved}); err == nil {
		t.Fatal("a channel admin moved their channel onto another host")
	}
	if got, err := r.UpdateChannel(global, model.UpdateChannelInput{ID: id, PublicURL: &moved}); err != nil || got.PublicURL != moved {
		t.Fatalf("a global admin moving the URL: %v, %v", got, err)
	}
	if _, err := r.SetChannelAsDefault(scoped, id); err == nil {
		t.Fatal("a channel admin made their channel the install default")
	}
	if _, err := r.DeleteChannel(scoped, id); err == nil {
		t.Fatal("a channel admin deleted a channel")
	}
	if ok, err := r.DeleteChannel(global, id); err != nil || !ok {
		t.Fatalf("a global admin deleting: %v, %v", ok, err)
	}
}
