package qna

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func seedProduct(t *testing.T, conn *sql.DB, channels ...uuid.UUID) uuid.UUID {
	t.Helper()
	for _, ch := range channels {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Q','USD',true)`, ch, "qna-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "qna-"+uuid.NewString()[:8]).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	sku := "qna-" + uuid.NewString()[:8]
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	return productID
}

type countingOutbox struct {
	mu    sync.Mutex
	types []string
}

func (o *countingOutbox) Enqueue(_ context.Context, _ *sql.Tx, e events.Event) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.types = append(o.types, e.Type())
	return nil
}

func (o *countingOutbox) count(eventType string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, t := range o.types {
		if t == eventType {
			n++
		}
	}
	return n
}

// Editing an answer, or answering twice at once, must not tell webhook subscribers the question was answered again
func TestOnlyTheFirstAnswerIsAnnounced(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	ch := uuid.New()
	productID := seedProduct(t, conn, ch)
	ob := &countingOutbox{}
	svc := NewService(db.New(conn))
	svc.SetOutbox(ob, txrunner.New(conn))

	q, err := svc.Submit(ctx, productID, ch, nil, "G", "g@example.test", "Как стирать эту футболку?")
	if err != nil {
		t.Fatal(err)
	}
	// the cap is in characters - 600 Cyrillic ones are 1200 bytes
	if _, err := svc.Submit(ctx, productID, ch, nil, "G", "", strings.Repeat("я", 600)); err != nil {
		t.Fatalf("a 600-character question refused: %v", err)
	}
	if ob.count(EventQuestionSubmitted) != 2 {
		t.Fatalf("submitted events: %d, want 1", ob.count(EventQuestionSubmitted))
	}

	holder, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(`SELECT 1 FROM product_questions WHERE id = $1 FOR UPDATE`, q.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Answer(ctx, q.ID, uuid.New(), "Холодная вода.")
		}(i)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var waiting int
		if err := conn.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d answers reached the row lock", waiting)
		}
	}
	_ = holder.Rollback()
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Answer(ctx, q.ID, uuid.New(), "Edited: cold water."); err != nil {
		t.Fatal(err)
	}
	if got := ob.count(EventQuestionAnswered); got != 1 {
		t.Errorf("answered events: %d, want 1", got)
	}
}

// Not found is for a row that isn't there - a database that can't answer must not read as one
func TestOnlyAMissingRowIsNotFound(t *testing.T) {
	conn := testdb.Require(t)
	svc := NewService(db.New(conn))
	if _, err := svc.Get(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing row: %v, want ErrNotFound", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.Reject(ctx, uuid.New()); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("a failed query read as %v, want a real error", err)
	}
}

func TestAStoreReadsOnlyItsOwnPublishedQuestions(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	productID := seedProduct(t, conn, a, b)
	svc := NewService(db.New(conn))
	for ch, text := range map[uuid.UUID]string{a: "asked on store A?", b: "asked on store B?"} {
		q, err := svc.Submit(ctx, productID, ch, nil, "G", "g@example.test", text)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Answer(ctx, q.ID, uuid.New(), "yes"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.Published(ctx, productID, b, 10)
	if err != nil || len(got) != 1 || got[0].Question != "asked on store B?" || got[0].ChannelID != b {
		t.Fatalf("store B read %+v, %v, want only its own question", got, err)
	}
}
