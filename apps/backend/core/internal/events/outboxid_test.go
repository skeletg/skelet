package events

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestAnEventPublishedDuringAnOutboxDispatchIsNotThatRow(t *testing.T) {
	b := NewBus()
	row := uuid.New()
	var parentSaw, childSyncSaw bool
	childAsync := make(chan bool, 1)
	b.SubscribeSync("child.sync", 0, func(ctx context.Context, _ Event) error {
		_, childSyncSaw = OutboxID(ctx)
		return nil
	})
	b.Subscribe("child.async", func(ctx context.Context, _ Event) error {
		_, ok := OutboxID(ctx)
		childAsync <- ok
		return nil
	})
	b.Subscribe("parent", func(ctx context.Context, _ Event) error {
		_, parentSaw = OutboxID(ctx)
		if err := b.PublishSync(ctx, &testEvent{name: "child.sync"}); err != nil {
			return err
		}
		b.Publish(ctx, &testEvent{name: "child.async"})
		return nil
	})
	if err := b.DispatchAsyncNow(WithOutboxID(context.Background(), row), &testEvent{name: "parent"}); err != nil {
		t.Fatal(err)
	}
	if !parentSaw {
		t.Fatal("the dispatched row's handler never saw its outbox id")
	}
	if childSyncSaw || <-childAsync {
		t.Fatal("a child event carried its parent's outbox id, so its webhook delivery would dedupe against the parent's")
	}
}
