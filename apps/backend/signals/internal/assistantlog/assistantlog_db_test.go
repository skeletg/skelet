package assistantlog

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func turn(conv, ch uuid.UUID, customer *uuid.UUID, anon, question string, at time.Time) db.RecordAssistantTurnParams {
	p := db.RecordAssistantTurnParams{ID: uuid.New(), ConversationID: conv, ChannelID: ch, AnonID: anon, Question: question, Reply: "ok", FollowUps: []string{}, Rounds: json.RawMessage(`[]`), At: at}
	if customer != nil {
		p.CustomerID = uuid.NullUUID{UUID: *customer, Valid: true}
	}
	return p
}

func record(t *testing.T, s *Store, p db.RecordAssistantTurnParams) bool {
	t.Helper()
	ok, err := s.Record(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestConversationsAreKeptPerStoreAndPersonAndGoWhole(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	ch, other := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	conv := uuid.New()

	first := turn(conv, ch, nil, "visitor-1", "Do you have 100% wool socks?", now.Add(-time.Hour))
	customer := uuid.New()
	second := turn(conv, ch, &customer, "visitor-1", "add two", now.Add(-30*time.Minute))
	second.CartChanged = true
	second.Rounds = json.RawMessage(`[{"thinking":"look it up","calls":[{"tool":"search_products","inputJson":"{}","output":"Socks"},{"tool":"get_product","isError":true}]},{"text":"Added.","shown":true}]`)
	record(t, s, second) // arriving first, the way a retried turn can
	if !record(t, s, first) || record(t, s, first) {
		t.Fatal("a turn wasn't kept once and only once")
	}
	if record(t, s, turn(conv, other, nil, "visitor-1", "hijack", now)) {
		t.Fatal("another store's turn landed in this conversation")
	}

	rows, more, err := s.List(ctx, ch, Query{})
	if err != nil || more || len(rows) != 1 {
		t.Fatalf("list = %+v, %v, %v", rows, more, err)
	}
	r := rows[0]
	if r.Turns != 2 || !r.CartChanged || r.Checkout || r.FirstQuestion != first.Question || !r.CustomerID.Valid || r.CustomerID.UUID != customer || !r.LastAt.Equal(second.At) {
		t.Errorf("conversation = %+v", r)
	}
	turns, err := s.Turns(ctx, ch, conv)
	if err != nil || len(turns) != 2 || turns[0].Question != first.Question {
		t.Fatalf("turns = %+v, %v", turns, err)
	}
	if tr := turns[1]; tr.Steps != 2 || tr.ToolCalls != 2 || !tr.Refused || !tr.Thought || turns[0].Steps != 0 || turns[0].Thought {
		t.Errorf("summed-up rounds = %+v", tr)
	}
	if raw, found, err := s.TurnRounds(ctx, ch, second.ID); err != nil || !found || string(raw) == "[]" {
		t.Errorf("rounds = %s, %v, %v", raw, found, err)
	}
	if _, found, _ := s.TurnRounds(ctx, other, second.ID); found {
		t.Error("another store read a turn's rounds")
	}
	if none, _ := s.Turns(ctx, other, conv); len(none) != 0 {
		t.Error("another store read this conversation")
	}
	if c, found, err := s.Conversation(ctx, ch, conv); err != nil || !found || c.Turns != 2 || c.AnonID != "visitor-1" || c.FirstQuestion != first.Question || !c.StartedAt.Equal(first.At) {
		t.Errorf("conversation = %+v, %v, %v", c, found, err)
	}
	if _, found, err := s.Conversation(ctx, other, conv); err != nil || found {
		t.Errorf("another store found this conversation: %v, %v", found, err)
	}

	for _, q := range []string{"100%", "0% wo", "WOOL"} {
		if found, _, _ := s.List(ctx, ch, Query{Search: q}); len(found) != 1 {
			t.Errorf("search %q found %d", q, len(found))
		}
	}
	for _, q := range []string{"1_0% wool", "wo%ks", "nothing like it"} {
		if found, _, _ := s.List(ctx, ch, Query{Search: q}); len(found) != 0 {
			t.Errorf("search %q matched as a pattern: %d", q, len(found))
		}
	}

	if err := s.Erase(ctx, []uuid.UUID{uuid.New()}, []string{"visitor-1"}); err != nil {
		t.Fatal(err)
	}
	if left, _, _ := s.List(ctx, ch, Query{}); len(left) != 0 {
		t.Error("erasing the visitor left their conversation")
	}
	if left, _ := s.Turns(ctx, ch, conv); len(left) != 0 {
		t.Error("the turns outlived their conversation")
	}
}

func TestErasingACustomerReachesTheirTurnsInSomeoneElsesConversation(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	ch, conv, theirs := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	a, b := uuid.New(), uuid.New()
	record(t, s, turn(conv, ch, &a, "shared-tab", "first from A", now.Add(-time.Hour)))
	fromB := turn(conv, ch, &b, "shared-tab", "then B signed in", now)
	record(t, s, fromB)
	record(t, s, turn(theirs, ch, &b, "", "B's own", now))

	if err := s.Erase(ctx, []uuid.UUID{b}, nil); err != nil {
		t.Fatal(err)
	}
	if left, _, _ := s.List(ctx, ch, Query{}); len(left) != 1 || left[0].ID != conv || left[0].Turns != 1 {
		t.Fatalf("after erasing B = %+v", left)
	}
	if _, found, _ := s.TurnRounds(ctx, ch, fromB.ID); found {
		t.Error("B's turn in A's conversation survived B's erasure")
	}
}

func TestTheListPagesNewestFirstAndAgesOutWholeConversations(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	ch := uuid.New()
	base := time.Now().UTC().Truncate(time.Microsecond)
	var convs []uuid.UUID
	for i := range 5 {
		c := uuid.New()
		convs = append(convs, c)
		record(t, s, turn(c, ch, nil, "v", "q", base.Add(-time.Duration(i)*24*time.Hour)))
	}
	page, more, err := s.List(ctx, ch, Query{Limit: 2})
	if err != nil || !more || len(page) != 2 || page[0].ID != convs[0] || page[1].ID != convs[1] {
		t.Fatalf("page 1 = %+v, more %v, %v", page, more, err)
	}
	last := page[1]
	page, more, _ = s.List(ctx, ch, Query{Limit: 2, BeforeAt: &last.LastAt, BeforeID: last.ID})
	if more == false || len(page) != 2 || page[0].ID != convs[2] {
		t.Fatalf("page 2 = %+v, more %v", page, more)
	}

	ongoing := convs[0]
	record(t, s, turn(ongoing, ch, nil, "v", "a question from long ago", base.Add(-30*24*time.Hour)))

	gone, old, err := s.Prune(ctx, base.Add(-36*time.Hour))
	if err != nil || gone < 3 || old < 1 {
		t.Fatalf("pruned %d conversations and %d turns, %v - want the quiet ones whole and the old turn", gone, old, err)
	}
	left, _, _ := s.List(ctx, ch, Query{})
	if len(left) != 2 {
		t.Fatalf("%d conversations left, want the two inside the window", len(left))
	}
	if left[0].ID != ongoing || left[0].Turns != 1 || left[0].FirstQuestion != "q" {
		t.Errorf("an old turn outlived the window inside a conversation still going: %+v", left[0])
	}
}

func TestSearchFindsAWordInAnyTurn(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	ch := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	hit, miss := uuid.New(), uuid.New()
	record(t, s, turn(hit, ch, nil, "v", "hello", now.Add(-time.Hour)))
	reply := turn(hit, ch, nil, "v", "and shipping?", now)
	reply.Reply = "We ship to Germany in three days."
	record(t, s, reply)
	record(t, s, turn(miss, ch, nil, "v", "hello", now))

	found, _, err := s.List(ctx, ch, Query{Search: "germany"})
	if err != nil || len(found) != 1 || found[0].ID != hit || found[0].FirstQuestion != "hello" {
		t.Errorf("search = %+v, %v", found, err)
	}
	if long, _, err := s.List(ctx, ch, Query{Search: strings.Repeat("ab", 500)}); err != nil || len(long) != 0 {
		t.Errorf("a very long search = %d, %v", len(long), err)
	}
	for _, short := range []string{" ge ", "a b", "...", "1 2"} {
		if all, _, err := s.List(ctx, ch, Query{Search: short}); err != nil || len(all) != 2 {
			t.Errorf("search %q = %d, %v - want it ignored, every conversation listed", short, len(all), err)
		}
	}
}

func TestErasingManyCustomersAtOnceGoesThroughEverySlice(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	ch := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	customers := make([]uuid.UUID, 2*eraseBatch+3)
	for i := range customers {
		customers[i] = uuid.New()
	}
	anon := make([]string, eraseBatch+5)
	for i := range anon {
		anon[i] = "erased-" + uuid.NewString()[:8]
	}
	first, middle, last := customers[0], customers[eraseBatch+1], customers[len(customers)-1]
	for _, c := range []uuid.UUID{first, middle, last} {
		record(t, s, turn(uuid.New(), ch, &c, "", "mine", now))
	}
	record(t, s, turn(uuid.New(), ch, nil, anon[len(anon)-1], "as a guest", now))
	kept := uuid.New()
	record(t, s, turn(kept, ch, nil, "someone-else", "not theirs", now))

	if err := s.Erase(ctx, customers, anon); err != nil {
		t.Fatal(err)
	}
	if left, _, err := s.List(ctx, ch, Query{}); err != nil || len(left) != 1 || left[0].ID != kept {
		t.Fatalf("after erasing %d customers and %d guest ids = %+v, %v - want only the bystander", len(customers), len(anon), left, err)
	}
}

func TestTheListFiltersToConversationsTheStoreCouldntAnswer(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	ch, other := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	add := func(conv uuid.UUID, question string, at time.Time, mutate func(*db.RecordAssistantTurnParams)) db.RecordAssistantTurnParams {
		p := turn(conv, ch, nil, "v", question, at)
		p.Recap = "Shown as cards: Mug (id x)"
		if mutate != nil {
			mutate(&p)
		}
		record(t, s, p)
		return p
	}
	fine, mixed, cut, shopper := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	add(fine, "mugs?", now.Add(-6*time.Minute), nil)
	add(mixed, "nothing came back", now.Add(-5*time.Minute), func(p *db.RecordAssistantTurnParams) { p.Reply, p.Recap = "", "" })
	add(mixed, "fine answer", now.Add(-4*time.Minute), nil)
	mars := add(mixed, "do you ship to Mars", now.Add(-3*time.Minute), func(p *db.RecordAssistantTurnParams) { p.Unsure, p.FoundNothing = true, true })
	add(cut, "stopped early", now.Add(-2*time.Minute), func(p *db.RecordAssistantTurnParams) {
		p.Incomplete, p.CustomerID = true, uuid.NullUUID{UUID: shopper, Valid: true}
	})
	record(t, s, func() db.RecordAssistantTurnParams {
		p := turn(uuid.New(), other, nil, "v", "another store's", now)
		p.Unsure = true
		return p
	}())

	ids := func(rows []db.ListAssistantConversationsRow) []uuid.UUID {
		out := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			out[i] = r.ID
		}
		return out
	}
	all, _, err := s.List(ctx, ch, Query{})
	if err != nil || !slices.Equal(ids(all), []uuid.UUID{cut, mixed, fine}) {
		t.Fatalf("every conversation = %v, %v", ids(all), err)
	}
	if m := all[1]; m.Unanswered != 2 || m.GapID != mars.ID || !m.GapUnsure || !m.GapFoundNothing || m.GapNoAnswer || m.GapQuestion != "do you ship to Mars" {
		t.Errorf("want the mixed one carrying two unanswered and the latest of them: %+v", m)
	}
	if f := all[2]; f.Unanswered != 0 || f.GapID != uuid.Nil {
		t.Errorf("an answered conversation carries none: %+v", f)
	}

	for gap, want := range map[string][]uuid.UUID{AnyGap: {cut, mixed}, "no_answer": {mixed}, "cut_short": {cut}, "found_nothing": {mixed}, "unsure": {mixed}} {
		rows, _, err := s.List(ctx, ch, Query{Unanswered: gap})
		if err != nil || !slices.Equal(ids(rows), want) {
			t.Errorf("%s = %v, %v, want %v", gap, ids(rows), err, want)
		}
	}
	if rows, _, _ := s.List(ctx, ch, Query{Unanswered: "no_answer"}); len(rows) != 1 || rows[0].Unanswered != 1 || rows[0].GapQuestion != "nothing came back" || !rows[0].GapNoAnswer {
		t.Errorf("filtered to one kind, the conversation should carry its latest of that kind: %+v", rows)
	}
	if rows, _, _ := s.List(ctx, ch, Query{Unanswered: AnyGap, Search: "mars"}); !slices.Equal(ids(rows), []uuid.UUID{mixed}) {
		t.Errorf("filter and search together = %v", ids(rows))
	}
	page, more, _ := s.List(ctx, ch, Query{Unanswered: AnyGap, Limit: 1})
	if !more || !slices.Equal(ids(page), []uuid.UUID{cut}) {
		t.Fatalf("first page = %v, more %v", ids(page), more)
	}
	if rest, more, _ := s.List(ctx, ch, Query{Unanswered: AnyGap, Limit: 1, BeforeAt: &page[0].LastAt, BeforeID: page[0].ID}); more || !slices.Equal(ids(rest), []uuid.UUID{mixed}) {
		t.Errorf("second page = %v, more %v", ids(rest), more)
	}

	counts, err := s.GapCounts(ctx, ch, nil)
	if err != nil || counts.Any != 2 || counts.NoAnswer != 1 || counts.CutShort != 1 || counts.FoundNothing != 1 || counts.Unsure != 1 {
		t.Errorf("counts = %+v, %v - conversations, each once, another store's left out", counts, err)
	}
	if theirs, err := s.GapCounts(ctx, ch, &shopper); err != nil || theirs.Any != 1 || theirs.CutShort != 1 || theirs.Unsure != 0 {
		t.Errorf("one shopper's counts = %+v, %v", theirs, err)
	}
	if one, found, err := s.Conversation(ctx, ch, mixed); err != nil || !found || one.Unanswered != 2 || one.GapID != mars.ID {
		t.Errorf("one conversation = %+v, %v", one, err)
	}
}
