package ai

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func historyDB(t *testing.T) (*History, *sql.DB, *db.Queries) {
	t.Helper()
	conn := testdb.Require(t)
	q := db.New(conn)
	return NewHistory(q), conn, q
}

func seedHistoryAdmin(t *testing.T, q *db.Queries) uuid.UUID {
	t.Helper()
	a, err := q.CreateAdminUser(context.Background(), db.CreateAdminUserParams{
		Email: "chat-" + uuid.NewString()[:8] + "@example.test", PasswordHash: "x", FirstName: "C", LastName: "H",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	t.Cleanup(func() { _ = q.DeleteAdminUser(context.Background(), a.ID) })
	return a.ID
}

func userMsg(s string) HistoryMessage { return HistoryMessage{Role: RoleUser, Content: s} }

func TestHistorySaveWithoutChannelRoundTrips(t *testing.T) {
	h, _, q := historyDB(t)
	ctx := context.Background()
	admin := seedHistoryAdmin(t, q)

	id, err := h.Save(ctx, nil, admin, "", []HistoryMessage{userMsg("how are sales?"), {Role: RoleAssistant, Content: "fine"}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := h.Get(ctx, id, admin)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "how are sales?" || len(got.Messages) != 2 || got.Messages[1].Content != "fine" {
		t.Fatalf("got %+v", got)
	}
	if _, err := h.Save(ctx, &id, admin, "renamed", []HistoryMessage{userMsg("x")}); err != nil {
		t.Fatalf("update own: %v", err)
	}
	if got, _ := h.Get(ctx, id, admin); got.Title != "renamed" || len(got.Messages) != 1 {
		t.Fatalf("after update %+v", got)
	}
}

func TestHistoryUpdateForeignOrMissingIsNotFound(t *testing.T) {
	h, _, q := historyDB(t)
	ctx := context.Background()
	owner, other := seedHistoryAdmin(t, q), seedHistoryAdmin(t, q)
	id, err := h.Save(ctx, nil, owner, "", []HistoryMessage{userMsg("mine")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Save(ctx, &id, other, "", []HistoryMessage{userMsg("theirs")}); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("foreign update err = %v", err)
	}
	missing := uuid.New()
	if _, err := h.Save(ctx, &missing, owner, "", nil); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("missing update err = %v", err)
	}
	if got, _ := h.Get(ctx, id, owner); got.Messages[0].Content != "mine" {
		t.Errorf("foreign update leaked: %+v", got)
	}
}

func TestHistoryGetMissingIsNotFound(t *testing.T) {
	h, conn, q := historyDB(t)
	ctx := context.Background()
	owner, other := seedHistoryAdmin(t, q), seedHistoryAdmin(t, q)
	if _, err := h.Get(ctx, uuid.New(), owner); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("missing get err = %v", err)
	}
	id, _ := h.Save(ctx, nil, owner, "", []HistoryMessage{userMsg("a")})
	if _, err := h.Get(ctx, id, other); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("foreign get err = %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE chat_conversations SET messages = '{"not":"an array"}' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Get(ctx, id, owner); err == nil || errors.Is(err, ErrConversationNotFound) {
		t.Errorf("corrupt messages err = %v, want a decode error", err)
	}
}

func TestHistoryListScopedAndLimited(t *testing.T) {
	h, _, q := historyDB(t)
	ctx := context.Background()
	admin, other := seedHistoryAdmin(t, q), seedHistoryAdmin(t, q)
	for range MaxHistoryLimit + 5 {
		if _, err := h.Save(ctx, nil, admin, "", []HistoryMessage{userMsg("q")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Save(ctx, nil, other, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ in, want int32 }{{0, DefaultHistoryLimit}, {-1, DefaultHistoryLimit}, {7, 7}, {MaxHistoryLimit + 1, MaxHistoryLimit}, {1000, MaxHistoryLimit}} {
		rows, err := h.List(ctx, admin, tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if int32(len(rows)) != tc.want {
			t.Errorf("limit %d returned %d, want %d", tc.in, len(rows), tc.want)
		}
	}
	rows, _ := h.List(ctx, other, 0)
	if len(rows) != 1 {
		t.Errorf("other admin sees %d rows, want 1", len(rows))
	}
}

func TestHistoryCaps(t *testing.T) {
	h, _, q := historyDB(t)
	ctx := context.Background()
	admin := seedHistoryAdmin(t, q)

	if _, err := h.Save(ctx, nil, admin, "", []HistoryMessage{userMsg(strings.Repeat("é", MaxMessageRunes+1))}); !errors.Is(err, ErrMessageTooLong) {
		t.Errorf("oversized content err = %v", err)
	}
	if _, err := h.Save(ctx, nil, admin, "", []HistoryMessage{userMsg(strings.Repeat("é", MaxMessageRunes))}); err != nil {
		t.Errorf("content at the cap: %v", err)
	}
	id, err := h.Save(ctx, nil, admin, strings.Repeat("t", MaxTitleRunes+50), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := h.Get(ctx, id, admin)
	if n := len([]rune(got.Title)); n != MaxTitleRunes {
		t.Errorf("title runes = %d, want %d", n, MaxTitleRunes)
	}

	msgs := make([]HistoryMessage, MaxHistoryMessages+10)
	for i := range msgs {
		msgs[i] = userMsg(time.Duration(i).String())
	}
	id, err = h.Save(ctx, nil, admin, "x", msgs)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = h.Get(ctx, id, admin)
	if len(got.Messages) != MaxHistoryMessages || got.Messages[0].Content != msgs[10].Content {
		t.Errorf("kept %d messages starting %q", len(got.Messages), got.Messages[0].Content)
	}
}

func TestHistoryRejectsUnknownRole(t *testing.T) {
	h, _, q := historyDB(t)
	admin := seedHistoryAdmin(t, q)
	for _, role := range []string{"system", "tool", ""} {
		_, err := h.Save(context.Background(), nil, admin, "", []HistoryMessage{userMsg("a"), {Role: role, Content: "b"}})
		if !errors.Is(err, ErrInvalidRole) {
			t.Errorf("role %q err = %v", role, err)
		}
	}
}

func TestConversationTitle(t *testing.T) {
	long := strings.Repeat("ж", DerivedTitleRunes+5)
	for _, tc := range []struct {
		title string
		msgs  []HistoryMessage
		want  string
	}{
		{"  Mine  ", []HistoryMessage{userMsg("ignored")}, "Mine"},
		{"", []HistoryMessage{{Role: RoleAssistant, Content: "hi"}, userMsg("  first q ")}, "first q"},
		{"", []HistoryMessage{userMsg(long)}, string([]rune(long)[:DerivedTitleRunes])},
		{"   ", []HistoryMessage{{Role: RoleAssistant, Content: "only me"}}, DefaultTitle},
		{"", nil, DefaultTitle},
	} {
		if got := conversationTitle(tc.title, tc.msgs); got != tc.want {
			t.Errorf("conversationTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestHistoryPruneBefore(t *testing.T) {
	h, conn, q := historyDB(t)
	ctx := context.Background()
	admin := seedHistoryAdmin(t, q)
	old, _ := h.Save(ctx, nil, admin, "old", nil)
	fresh, _ := h.Save(ctx, nil, admin, "fresh", nil)
	if _, err := conn.ExecContext(ctx, `UPDATE chat_conversations SET updated_at = now() - interval '40 days' WHERE id = $1`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PruneBefore(ctx, time.Now().AddDate(0, 0, -30)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Get(ctx, old, admin); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("old row survived prune: %v", err)
	}
	if _, err := h.Get(ctx, fresh, admin); err != nil {
		t.Errorf("fresh row pruned: %v", err)
	}
}

func TestHistoryCascadesWithAdmin(t *testing.T) {
	h, _, q := historyDB(t)
	ctx := context.Background()
	admin := seedHistoryAdmin(t, q)
	id, err := h.Save(ctx, nil, admin, "", []HistoryMessage{userMsg("bye")})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteAdminUser(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Get(ctx, id, admin); !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("conversation outlived its admin: %v", err)
	}
	if _, err := h.Save(ctx, nil, uuid.New(), "", nil); err == nil {
		t.Error("insert for an unknown admin succeeded, want an FK violation")
	}
}
