package lens

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestChunkBlobKeyIsDeterministicPerSequence(t *testing.T) { // replay chunks are re-uploaded on retry, and a key that drifted would leave orphaned objects the prune path can never find
	sess := uuid.New()
	first := chunkBlobKey(sess, 0)

	if first != chunkBlobKey(sess, 0) {
		t.Error("two calls with the same session and sequence produced different keys")
	}
	if first == chunkBlobKey(sess, 1) {
		t.Error("sequence 0 and 1 share a key, so the second chunk would overwrite the first")
	}
	if chunkBlobKey(uuid.New(), 0) == first {
		t.Error("two sessions share a chunk key")
	}
	if !strings.HasPrefix(first, "lens/replay/") || !strings.HasSuffix(first, ".gz") {
		t.Errorf("key = %q, want a lens/replay prefix and a .gz suffix", first)
	}
	if !strings.Contains(first, sess.String()) {
		t.Errorf("key = %q, want the session id in it so an operator can find a recording's objects", first)
	}
}

func TestSnapshotBlobKeyContainsAPageKeyWithSlashes(t *testing.T) { // page keys are URL paths, and passing one through unhashed would fan a single channel's snapshots across a directory tree the prune path walks blindly
	ch := uuid.New()
	key := snapshotBlobKey(ch, "/collections/winter/tires?page=2", "1440x900", "default", "abc123")

	if got := strings.Count(key, "/"); got != 3 { // lens/snapshot/<channel>/<hash>.gz
		t.Errorf("key = %q has %d slashes, want the page key folded into one hashed segment", key, got)
	}
	if strings.Contains(key, "collections") {
		t.Errorf("key = %q, want the page key hashed rather than embedded", key)
	}
	if !strings.HasPrefix(key, "lens/snapshot/"+ch.String()+"/") {
		t.Errorf("key = %q, want it scoped under the channel", key)
	}
}

func TestSnapshotBlobKeyIsDeterministicAndVariesWithEveryPart(t *testing.T) { // the key is recomputed rather than stored, so any part it ignores would silently serve one page's snapshot for another
	ch := uuid.New()
	base := snapshotBlobKey(ch, "/p/tire", "1440x900", "default", "hash1")
	if base != snapshotBlobKey(ch, "/p/tire", "1440x900", "default", "hash1") {
		t.Fatal("the same inputs produced two different keys")
	}

	for name, key := range map[string]string{
		"page key":   snapshotBlobKey(ch, "/p/other", "1440x900", "default", "hash1"),
		"viewport":   snapshotBlobKey(ch, "/p/tire", "390x844", "default", "hash1"),
		"state":      snapshotBlobKey(ch, "/p/tire", "1440x900", "cart-open", "hash1"),
		"layoutHash": snapshotBlobKey(ch, "/p/tire", "1440x900", "default", "hash2"),
		"channel":    snapshotBlobKey(uuid.New(), "/p/tire", "1440x900", "default", "hash1"),
	} {
		if key == base {
			t.Errorf("changing the %s did not change the key", name)
		}
	}
}

func TestStateOrNamesTheDefaultState(t *testing.T) { // state is part of the snapshot primary key, and an empty string there would split "no state" from "default" into two rows for the same page
	if got := stateOr(""); got != "default" {
		t.Errorf("stateOr(%q) = %q, want default", "", got)
	}
	for _, in := range []string{"default", "cart-open", "menu"} {
		if got := stateOr(in); got != in {
			t.Errorf("stateOr(%q) = %q, want it passed through", in, got)
		}
	}
}

func TestNormalizeQueryGroupsEquivalentSearches(t *testing.T) { // the search rollup keys on this, so casing or spacing differences would scatter one term across several rows
	for in, want := range map[string]string{
		"Red Shoes":     "red shoes",
		"red  shoes":    "red shoes",
		"  RED shoes  ": "red shoes",
		"red\tshoes":    "red shoes",
		"red\n\nshoes":  "red shoes",
		"RED":           "red",
		"":              "",
		"   ":           "",
		"205/55 R16":    "205/55 r16",
		"Ünïcode Térm":  "ünïcode térm",
	} {
		if got := normalizeQuery(in); got != want {
			t.Errorf("normalizeQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeQueryCapsRunesNotBytes(t *testing.T) { // a byte cap would slice a multibyte rune in half and write invalid UTF-8 into the rollup key
	long := strings.Repeat("é", 200)
	got := normalizeQuery(long)

	if n := len([]rune(got)); n != 80 {
		t.Errorf("%d runes, want the 80-rune cap", n)
	}
	if !utf8.ValidString(got) {
		t.Errorf("result is not valid UTF-8: %q", got)
	}
	if short := normalizeQuery("tires"); short != "tires" {
		t.Errorf("a short query was altered: %q", short)
	}
}

func TestPropStringOnlyAcceptsStrings(t *testing.T) { // event props are free-form JSON off a public endpoint, so a number where a string was expected must read as absent rather than panic
	if got := propString("term"); got != "term" {
		t.Errorf("propString(string) = %q, want term", got)
	}
	for name, v := range map[string]any{
		"nil":     nil,
		"number":  float64(3),
		"bool":    true,
		"object":  map[string]any{"a": 1},
		"array":   []any{"a"},
		"pointer": new(string),
	} {
		if got := propString(v); got != "" {
			t.Errorf("propString(%s) = %q, want empty", name, got)
		}
	}
}

func TestResultsZeroTreatsUnknownValuesAsNonZero(t *testing.T) { // a zero-result search is a merchandising signal, and guessing at an unparseable count would invent one
	for name, v := range map[string]any{
		"json number zero": float64(0),
		"int zero":         0,
		"int64 zero":       int64(0),
	} {
		if !resultsZero(v) {
			t.Errorf("resultsZero(%s) = false, want true", name)
		}
	}
	for name, v := range map[string]any{
		"json number":     float64(7),
		"int":             3,
		"int64":           int64(1),
		"string zero":     "0",
		"nil":             nil,
		"bool":            false,
		"negative":        float64(-1),
		"stringy nothing": "",
	} {
		if resultsZero(v) {
			t.Errorf("resultsZero(%s) = true, want false so no zero-result signal is invented", name)
		}
	}
}

func TestOrEventIDMintsOneOnlyWhenMissing(t *testing.T) { // the id is the dedup key for at-least-once ingest, so overwriting a client-supplied one would let a retry double-count
	if got := orEventID("client-supplied-id"); got != "client-supplied-id" {
		t.Errorf("orEventID = %q, want the client's id kept", got)
	}

	first, second := orEventID(""), orEventID("")
	if first == "" || second == "" {
		t.Fatal("orEventID(\"\") returned an empty id")
	}
	if first == second {
		t.Error("two minted ids are identical, so unrelated events would dedup against each other")
	}
	if _, err := uuid.Parse(first); err != nil {
		t.Errorf("minted id %q is not a uuid: %v", first, err)
	}
}

func TestAsStringUnwrapsWhatTheDriverReturns(t *testing.T) { // sqlc types COALESCE and jsonb projections as any, and lib/pq hands them back as either string or []byte depending on the column
	for name, tc := range map[string]struct {
		in   any
		want string
	}{
		"string":      {"/p/tire", "/p/tire"},
		"bytes":       {[]byte("/p/tire"), "/p/tire"},
		"empty bytes": {[]byte{}, ""},
		"nil":         {nil, ""},
		"number":      {int64(3), ""},
		"bool":        {true, ""},
	} {
		if got := asString(tc.in); got != tc.want {
			t.Errorf("asString(%s) = %q, want %q", name, got, tc.want)
		}
	}
}

func TestMetaFromComputesTheDuration(t *testing.T) {
	id := uuid.New()
	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)

	m := metaFrom(id.String(), "anon-1", uuid.NullUUID{}, start, end, 42, "/", "/checkout", "1440x900", "curl/8")
	if m.ID != id {
		t.Errorf("ID = %s, want %s", m.ID, id)
	}
	if m.DurationMs != 90_000 {
		t.Errorf("DurationMs = %d, want 90000", m.DurationMs)
	}
	if m.CustomerID != nil {
		t.Errorf("CustomerID = %v for an anonymous session, want nil", m.CustomerID)
	}
	if m.AnonID != "anon-1" || m.EntryPage != "/" || m.ExitPage != "/checkout" || m.EventCount != 42 {
		t.Errorf("meta = %+v, want the passed-through fields intact", m)
	}
}

func TestMetaFromResolvesTheCustomer(t *testing.T) { // the player shows who the session belonged to, and a shared pointer or dropped id would mislabel it
	cust := uuid.New()
	m := metaFrom(uuid.NewString(), "anon-1", uuid.NullUUID{UUID: cust, Valid: true}, time.Now(), time.Now(), 1, "/", "/", "", "")
	if m.CustomerID == nil {
		t.Fatal("CustomerID = nil for a logged-in session")
	}
	if *m.CustomerID != cust {
		t.Errorf("CustomerID = %s, want %s", *m.CustomerID, cust)
	}
}

func TestMetaFromOnAnUnparseableID(t *testing.T) { // the id arrives from a row projection, and a bad one must leave uuid.Nil rather than abort the whole listing
	m := metaFrom("not-a-uuid", "anon", uuid.NullUUID{}, time.Now(), time.Now(), 0, "", "", "", "")
	if m.ID != uuid.Nil {
		t.Errorf("ID = %s, want uuid.Nil", m.ID)
	}
}

// stubSettings answers reads from a path→value map, at whatever channel was asked for
type stubSettings struct{ values map[string]string }

func (s stubSettings) Int(_ context.Context, path string, _ *uuid.UUID) (int64, error) {
	v, ok := s.values[path]
	if !ok {
		return 0, errors.New("unset")
	}
	return strconv.ParseInt(v, 10, 64)
}

func (s stubSettings) Bool(_ context.Context, path string, _ *uuid.UUID) (bool, error) {
	v, ok := s.values[path]
	if !ok {
		return false, errors.New("unset")
	}
	return strconv.ParseBool(v)
}

func (s stubSettings) String(_ context.Context, path string, _ *uuid.UUID) (string, error) {
	v, ok := s.values[path]
	if !ok {
		return "", errors.New("unset")
	}
	return v, nil
}

func lensWithConfig(values map[string]string) *Service {
	return &Service{cfg: stubSettings{values: values}}
}

func TestIngestConfigDefaultsAreTrackingOutOfTheBox(t *testing.T) { // wiring can hand the service no config at all, and a zero-valued response would switch the storefront tracker off on a fresh install
	got := (&Service{}).IngestConfig(context.Background(), uuid.New())
	want := ClientConfig{Enabled: true, SamplingRate: 100, CaptureMoves: true, ReplayEnabled: true, ReplaySamplingRate: 10, ReplayMaxMinutes: 30}
	if got != want {
		t.Errorf("IngestConfig with no config service = %+v, want %+v", got, want)
	}
}

func TestIngestConfigFallbacksMatchTheRegistryDefaults(t *testing.T) { // two copies of the same default drift apart - an operator who resets a knob would get the registry value while a nil-config install keeps the old one
	fallback := (&Service{}).IngestConfig(context.Background(), uuid.New())

	for path, want := range map[string]string{
		"signals/lens/enabled":              boolStr(fallback.Enabled),
		"signals/lens/sampling_rate":        intStr(fallback.SamplingRate),
		"signals/lens/capture_moves":        boolStr(fallback.CaptureMoves),
		"signals/lens/replay_enabled":       boolStr(fallback.ReplayEnabled),
		"signals/lens/replay_sampling_rate": intStr(fallback.ReplaySamplingRate),
		"signals/lens/replay_max_minutes":   intStr(fallback.ReplayMaxMinutes),
	} {
		def, ok := config.Find(path)
		if !ok {
			t.Errorf("%s is not registered", path)
			continue
		}
		if def.Default != want {
			t.Errorf("%s: registry default %q, but the tracker falls back to %q", path, def.Default, want)
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func intStr(n int) string { return strconv.Itoa(n) }

func TestIngestConfigAppliesTheChannelsStoredValues(t *testing.T) {
	svc := lensWithConfig(map[string]string{
		"signals/lens/enabled":              "false",
		"signals/lens/sampling_rate":        "25",
		"signals/lens/capture_moves":        "false",
		"signals/lens/replay_enabled":       "false",
		"signals/lens/replay_sampling_rate": "0",
		"signals/lens/replay_max_minutes":   "0",
	})

	got := svc.IngestConfig(context.Background(), uuid.New())
	want := ClientConfig{Enabled: false, SamplingRate: 25, CaptureMoves: false, ReplayEnabled: false, ReplaySamplingRate: 0, ReplayMaxMinutes: 0}
	if got != want {
		t.Errorf("IngestConfig = %+v, want %+v", got, want)
	}
}

func TestIngestConfigKeepsDefaultsOnUnusableStoredValues(t *testing.T) { // a row written by an older build, or hand-edited, must not switch tracking off or set a nonsense sample rate
	svc := lensWithConfig(map[string]string{
		"signals/lens/enabled":              "yes please",
		"signals/lens/sampling_rate":        "-5",
		"signals/lens/replay_sampling_rate": "not a number",
		"signals/lens/replay_max_minutes":   "-1",
	})

	got := svc.IngestConfig(context.Background(), uuid.New())
	if !got.Enabled {
		t.Error("Enabled = false from an unparseable value, want the default")
	}
	if got.SamplingRate != 100 {
		t.Errorf("SamplingRate = %d from a negative stored value, want the default 100", got.SamplingRate)
	}
	if got.ReplaySamplingRate != 10 {
		t.Errorf("ReplaySamplingRate = %d, want the default 10", got.ReplaySamplingRate)
	}
	if got.ReplayMaxMinutes != 30 {
		t.Errorf("ReplayMaxMinutes = %d from a negative stored value, want the default 30", got.ReplayMaxMinutes)
	}
}

func TestCaptureMovesDefaultsOnAndFailsOpen(t *testing.T) { // the move firehose is checked server-side per batch, and a read failure must not silently stop capturing
	if !(&Service{}).captureMoves(context.Background(), uuid.New()) {
		t.Error("captureMoves with no config service = false, want true")
	}
	if !lensWithConfig(map[string]string{"signals/lens/capture_moves": "maybe"}).captureMoves(context.Background(), uuid.New()) {
		t.Error("captureMoves on an unparseable value = false, want the permissive default")
	}
	if lensWithConfig(map[string]string{"signals/lens/capture_moves": "false"}).captureMoves(context.Background(), uuid.New()) {
		t.Error("captureMoves = true with the switch off")
	}
}
