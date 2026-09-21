package analytics

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// All tests here assert on the in-memory maps only and never call Flush (which
// touches the DB), so a nil *db.Queries is fine
func newTestRollup() *TrafficRollup { return NewTrafficRollup(nil) }

var (
	testDay = time.Date(2026, 6, 26, 10, 30, 0, 0, time.UTC)
	ch1     = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ch2     = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func TestAddHumanViewCoalesces(t *testing.T) {
	r := newTestRollup()
	r.AddHumanView(ch1, "/p/1", "mobile", "google", "anon-1", testDay)
	r.AddHumanView(ch1, "/p/1", "mobile", "google", "anon-1", testDay)
	r.AddHumanView(ch1, "/p/2", "mobile", "google", "anon-1", testDay)

	day := dayKey(testDay)
	if got := r.counts[rollupKey{channelID: ch1, day: day, kind: rollupKindHuman}]; got != 3 {
		t.Errorf("human counter = %d, want 3", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "total", ""}]; got != 3 {
		t.Errorf("total views = %d, want 3", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "page", "/p/1"}]; got != 2 {
		t.Errorf("page /p/1 views = %d, want 2", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "page", "/p/2"}]; got != 1 {
		t.Errorf("page /p/2 views = %d, want 1", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "device", "mobile"}]; got != 3 {
		t.Errorf("device mobile views = %d, want 3", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "referrer", "google"}]; got != 3 {
		t.Errorf("referrer google views = %d, want 3", got)
	}
}

func TestAddHumanViewEmptyDeviceReferrerDefaults(t *testing.T) {
	r := newTestRollup()
	r.AddHumanView(ch1, "/p", "", "", "anon-1", testDay)

	day := dayKey(testDay)
	if got := r.lensViews[lensKey{ch1, day, "device", "unknown"}]; got != 1 {
		t.Errorf("empty device should map to 'unknown', got %d under 'unknown'", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "referrer", "(direct)"}]; got != 1 {
		t.Errorf("empty referrer should map to '(direct)', got %d under '(direct)'", got)
	}
}

func TestAddHumanViewLastSeenKeepsMax(t *testing.T) {
	r := newTestRollup()
	earlier := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	later := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)

	// Call out of order: later first, then earlier - the MAX must stick
	r.AddHumanView(ch1, "/p", "d", "r", "anon-1", later)
	r.AddHumanView(ch1, "/p", "d", "r", "anon-1", earlier)

	if got := r.seen[seenKey{ch1, "anon-1"}]; !got.Equal(later) {
		t.Errorf("last-seen = %v, want the later timestamp %v", got, later)
	}
}

func TestAddHumanViewNilChannelNoOp(t *testing.T) {
	r := newTestRollup()
	r.AddHumanView(uuid.Nil, "/p", "d", "r", "anon-1", testDay)
	if len(r.counts) != 0 || len(r.lensViews) != 0 || len(r.seen) != 0 {
		t.Errorf("nil channel should be a no-op; counts=%d lens=%d seen=%d",
			len(r.counts), len(r.lensViews), len(r.seen))
	}
}

func TestAddHumanViewEmptyAnonSkipsSeen(t *testing.T) {
	r := newTestRollup()
	r.AddHumanView(ch1, "/p", "d", "r", "", testDay)
	if len(r.seen) != 0 {
		t.Errorf("empty anonID should not record a last-seen, got %d entries", len(r.seen))
	}
	// But the view counters still fire
	if got := r.lensViews[lensKey{ch1, dayKey(testDay), "total", ""}]; got != 1 {
		t.Errorf("view should still count with empty anonID, total=%d", got)
	}
}

func TestAddBrowseKeepsMostRecent(t *testing.T) {
	r := newTestRollup()
	earlier := time.Date(2026, 6, 26, 8, 0, 0, 0, time.UTC)
	later := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)

	r.AddBrowse(ch1, "anon-1", "/p/old", "SKU-OLD", "Old", "old.jpg", earlier)
	r.AddBrowse(ch1, "anon-1", "/p/new", "SKU-NEW", "New", "new.jpg", later)
	// An out-of-order older browse must NOT overwrite the most-recent one
	r.AddBrowse(ch1, "anon-1", "/p/older", "SKU-OLDER", "Older", "older.jpg", earlier.Add(-time.Hour))

	v, ok := r.browse[browseKey{ch1, "anon-1"}]
	if !ok {
		t.Fatal("browse not recorded")
	}
	if !v.at.Equal(later) || v.pageKey != "/p/new" || v.sku != "SKU-NEW" || v.name != "New" || v.image != "new.jpg" {
		t.Errorf("browse should keep the most-recent entry, got %+v", v)
	}
}

func TestAddBrowseFirstWriteWins(t *testing.T) {
	r := newTestRollup()
	r.AddBrowse(ch1, "anon-1", "/p/a", "A", "A", "a.jpg", testDay)
	v := r.browse[browseKey{ch1, "anon-1"}]
	if v.pageKey != "/p/a" {
		t.Errorf("first browse not recorded, got %+v", v)
	}
}

func TestAddBrowseNilChannelOrEmptyAnonNoOp(t *testing.T) {
	r := newTestRollup()
	r.AddBrowse(uuid.Nil, "anon-1", "/p", "S", "N", "i.jpg", testDay)
	r.AddBrowse(ch1, "", "/p", "S", "N", "i.jpg", testDay)
	if len(r.browse) != 0 {
		t.Errorf("nil channel / empty anon should be no-ops, got %d entries", len(r.browse))
	}
}

func TestAddInteractionCoalescesAndDefaults(t *testing.T) {
	r := newTestRollup()
	r.AddInteraction(ch1, "signup_click", testDay)
	r.AddInteraction(ch1, "signup_click", testDay)
	r.AddInteraction(ch1, "", testDay) // empty → "(unnamed)"

	day := dayKey(testDay)
	if got := r.lensViews[lensKey{ch1, day, "interaction", "signup_click"}]; got != 2 {
		t.Errorf("named interaction count = %d, want 2", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "interaction", "(unnamed)"}]; got != 1 {
		t.Errorf("empty name should map to '(unnamed)', got %d", got)
	}
}

func TestAddInteractionNilChannelNoOp(t *testing.T) {
	r := newTestRollup()
	r.AddInteraction(uuid.Nil, "x", testDay)
	if len(r.lensViews) != 0 {
		t.Errorf("nil channel interaction should be a no-op, got %d entries", len(r.lensViews))
	}
}

func TestAddSignalCoalescesAndDefaults(t *testing.T) {
	r := newTestRollup()
	r.AddSignal(ch1, "rage_click", "/p/1", testDay)
	r.AddSignal(ch1, "rage_click", "/p/1", testDay)
	r.AddSignal(ch1, "js_error", "", testDay) // empty key → "(none)"

	day := dayKey(testDay)
	if got := r.lensViews[lensKey{ch1, day, "rage_click", "/p/1"}]; got != 2 {
		t.Errorf("rage_click /p/1 = %d, want 2", got)
	}
	if got := r.lensViews[lensKey{ch1, day, "js_error", "(none)"}]; got != 1 {
		t.Errorf("empty key should map to '(none)', got %d", got)
	}
}

func TestAddSignalNilChannelOrEmptyDimNoOp(t *testing.T) {
	r := newTestRollup()
	r.AddSignal(uuid.Nil, "rage_click", "/p", testDay)
	r.AddSignal(ch1, "", "/p", testDay) // empty dim → no-op
	if len(r.lensViews) != 0 {
		t.Errorf("nil channel / empty dim should be no-ops, got %d entries", len(r.lensViews))
	}
}

func TestKeysAreChannelScoped(t *testing.T) {
	r := newTestRollup()
	r.AddHumanView(ch1, "/p", "d", "r", "a1", testDay)
	r.AddHumanView(ch2, "/p", "d", "r", "a1", testDay)

	day := dayKey(testDay)
	if got := r.counts[rollupKey{channelID: ch1, day: day, kind: rollupKindHuman}]; got != 1 {
		t.Errorf("ch1 human counter = %d, want 1", got)
	}
	if got := r.counts[rollupKey{channelID: ch2, day: day, kind: rollupKindHuman}]; got != 1 {
		t.Errorf("ch2 human counter = %d, want 1 (separate channel)", got)
	}
}
