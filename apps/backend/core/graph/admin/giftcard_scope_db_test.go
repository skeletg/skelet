package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestAChannelAdminManagesOnlyThatChannelsGiftCards(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	mine := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'GC','USD',true)`, mine, "gc-"+mine.String()[:8]); err != nil {
		t.Fatal(err)
	}
	svc := giftcard.NewService(giftcard.NewRepository(db.New(conn)), txrunner.New(conn))
	own, err := svc.Issue(ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000, ChannelID: &mine})
	if err != nil {
		t.Fatal(err)
	}
	everywhere, err := svc.Issue(ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	r := &Resolver{GiftcardSvc: svc}
	scoped := adminWithCachedGrants(auth.Grant{Slug: "giftcards.manage", ChannelID: &mine})

	if _, err := (&mutationResolver{r}).IssueGiftCard(scoped, "USD", 500, nil, nil, nil); err == nil {
		t.Fatal("a channel admin minted a card that spends on every channel")
	}
	if _, err := (&mutationResolver{r}).AdjustGiftCard(scoped, everywhere.ID.String(), 500, nil); err == nil {
		t.Fatal("a channel admin topped up a card that spends on every channel")
	}
	if _, err := (&mutationResolver{r}).AdjustGiftCard(scoped, own.ID.String(), 500, nil); err != nil {
		t.Fatalf("own channel's card: %v", err)
	}
	if _, err := (&queryResolver{r}).GiftCard(scoped, everywhere.Code); err == nil {
		t.Fatal("a channel admin read a card outside their channel")
	}
	list, err := (&queryResolver{r}).GiftCards(scoped, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.Code == everywhere.Code {
			t.Fatal("the list showed a card outside the admin's channel")
		}
	}
	if len(list) != 1 || list[0].Code != own.Code {
		t.Fatalf("list = %d cards, want only the channel's own", len(list))
	}
}

func TestAChannelAdminIssuesACardForTheirOwnStoreInItsCurrency(t *testing.T) {
	conn := testdb.Require(t)
	mine, other := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{mine, other} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'GC','USD',true)`, ch, "gi-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(conn)
	svc := giftcard.NewService(giftcard.NewRepository(q), txrunner.New(conn))
	r := &mutationResolver{&Resolver{GiftcardSvc: svc, ChannelSvc: channel.NewService(channel.NewRepository(q, txrunner.New(conn)))}}
	scoped := adminWithCachedGrants(auth.Grant{Slug: "giftcards.manage", ChannelID: &mine})
	id, otherID := mine.String(), other.String()

	card, err := r.IssueGiftCard(scoped, "USD", 500, nil, nil, &id)
	if err != nil {
		t.Fatalf("issuing on the admin's own store: %v", err)
	}
	if card.ChannelID == nil || *card.ChannelID != id {
		t.Fatalf("card channel = %v, want the store it was issued for - otherwise it spends on every store", card.ChannelID)
	}
	if _, err := r.IssueGiftCard(scoped, "USD", 500, nil, nil, &otherID); err == nil {
		t.Fatal("a channel admin issued a card for another store")
	}
	if _, err := r.IssueGiftCard(scoped, "EUR", 500, nil, nil, &id); err == nil {
		t.Fatal("a card was issued in a currency its store can never spend")
	}
}
