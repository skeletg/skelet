package adminprefs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeRepo struct {
	rows map[string]json.RawMessage // key = adminID|key
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]json.RawMessage{}} }

func k(adminID uuid.UUID, key string) string { return adminID.String() + "|" + key }

func (r *fakeRepo) List(_ context.Context, adminID uuid.UUID) ([]Preference, error) {
	var out []Preference
	for rk, v := range r.rows {
		if key, ok := strings.CutPrefix(rk, adminID.String()+"|"); ok {
			out = append(out, Preference{Key: key, Value: v})
		}
	}
	return out, nil
}

func (r *fakeRepo) Get(_ context.Context, adminID uuid.UUID, key string) (json.RawMessage, error) {
	return r.rows[k(adminID, key)], nil
}

func (r *fakeRepo) Set(_ context.Context, adminID uuid.UUID, key string, value json.RawMessage) error {
	r.rows[k(adminID, key)] = value
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, adminID uuid.UUID, key string) error {
	delete(r.rows, k(adminID, key))
	return nil
}

func TestSetThenListRoundtrips(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	admin := uuid.New()

	if err := svc.Set(context.Background(), admin, "nav:pins", []string{"/orders", "/webhooks"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	prefs, err := svc.List(context.Background(), admin)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(prefs) != 1 || prefs[0].Key != "nav:pins" {
		t.Fatalf("prefs = %+v, want the one key back", prefs)
	}
	var pins []string
	if err := json.Unmarshal(prefs[0].Value, &pins); err != nil || len(pins) != 2 {
		t.Errorf("stored value = %s, want the two pins as JSON", prefs[0].Value)
	}
}

// The mutation exposes one call for both write and clear, so nil has to mean delete
func TestNilValueDeletes(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	admin := uuid.New()

	if err := svc.Set(context.Background(), admin, "home:layout", map[string]any{"hidden": []string{"stats"}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := svc.Set(context.Background(), admin, "home:layout", nil); err != nil {
		t.Fatalf("Set nil: %v", err)
	}
	prefs, _ := svc.List(context.Background(), admin)
	if len(prefs) != 0 {
		t.Errorf("prefs after nil-set = %+v, want the key gone", prefs)
	}
}

func TestRejectsBadKeys(t *testing.T) {
	svc := NewService(newFakeRepo())
	for name, key := range map[string]string{
		"empty":         "",
		"whitespace":    "table: products",
		"leading colon": ":pins",
		"too long":      strings.Repeat("k", 129),
		"newline":       "nav\npins",
	} {
		if err := svc.Set(context.Background(), uuid.New(), key, "x"); err == nil {
			t.Errorf("a %s key was accepted, want it refused", name)
		}
	}
}

func TestRejectsOversizeValues(t *testing.T) {
	svc := NewService(newFakeRepo())
	big := strings.Repeat("x", 33<<10)
	if err := svc.Set(context.Background(), uuid.New(), "table:orders", big); err == nil {
		t.Error("a 33KB value was accepted, want it refused")
	}
}

// Preferences are strictly self-scoped - one admin's rows must never surface for another
func TestListIsPerAdmin(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	a, b := uuid.New(), uuid.New()
	_ = svc.Set(context.Background(), a, "nav:pins", []string{"/orders"})

	prefs, _ := svc.List(context.Background(), b)
	if len(prefs) != 0 {
		t.Errorf("admin B sees %+v, want nothing", prefs)
	}
}

// The inbox reads a muted-categories list this way on every query, and the value is a blob the
// dashboard owns - anything but a string array has to read as "nothing muted", never as an error
func TestStringsReadsAnArrayAndShrugsOffAnythingElse(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	admin := uuid.New()

	got, err := svc.Strings(context.Background(), admin, "notif:mutedCategories")
	if err != nil || got != nil {
		t.Fatalf("unset key: got %v, %v - want nil, nil", got, err)
	}

	if err := svc.Set(context.Background(), admin, "notif:mutedCategories", []string{"review", "form"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err = svc.Strings(context.Background(), admin, "notif:mutedCategories")
	if err != nil {
		t.Fatalf("strings: %v", err)
	}
	if len(got) != 2 || got[0] != "review" || got[1] != "form" {
		t.Errorf("got %v, want [review form]", got)
	}

	for _, junk := range []any{"review", 7, map[string]string{"a": "b"}} {
		if err := svc.Set(context.Background(), admin, "notif:mutedCategories", junk); err != nil {
			t.Fatalf("set %v: %v", junk, err)
		}
		got, err := svc.Strings(context.Background(), admin, "notif:mutedCategories")
		if err != nil || got != nil {
			t.Errorf("value %v: got %v, %v - want nil, nil", junk, got, err)
		}
	}
}
