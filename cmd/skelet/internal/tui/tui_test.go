package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

const (
	up    = "\x1b[A"
	down  = "\x1b[B"
	right = "\x1b[C"
)

func ui(keys string) (*UI, *strings.Builder, *int) {
	var out strings.Builder
	restored := 0
	raw := func() (func(), error) { return func() { restored++ }, nil }
	return New(strings.NewReader(keys), &out, raw, 60), &out, &restored
}

func TestConfirm(t *testing.T) {
	for keys, want := range map[string]bool{"y": true, "N": false, "\r": true, right + "\r": false, right + right + "\r": true} {
		u, out, restored := ui(keys)
		got, err := u.Confirm("Create one now?", true)
		if err != nil || got != want || *restored != 1 {
			t.Errorf("%q: %v %v, restored %d", keys, got, err, *restored)
		}
		if !strings.Contains(out.String(), "✓") || !strings.Contains(out.String(), "Create one now?") {
			t.Errorf("%q: no collapsed answer in %q", keys, out.String())
		}
	}
}

func TestSelect(t *testing.T) {
	opts := []Option{{"dev", "local"}, {"small", ""}, {"prod", ""}}
	for keys, want := range map[string]int{"\r": 1, down + "\r": 2, down + down + "\r": 0, up + "\r": 0, "3": 2, "j\r": 2} {
		u, _, _ := ui(keys)
		if got, err := u.Select("Target", opts, 1); err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", keys, got, err, want)
		}
	}
}

func TestMulti(t *testing.T) {
	opts := []Option{{"dashboard", ""}, {"shop", ""}, {"vetrina", "template"}}
	cases := map[string][]bool{
		"\r":                      {true, true, false},
		" \r":                     {false, true, false},
		down + down + " \r":       {true, true, true},
		"a\r":                     {true, true, true},
		"aa\r":                    {false, false, false},
		down + "x" + down + " \r": {true, false, true},
	}
	for keys, want := range cases {
		u, out, _ := ui(keys)
		got, err := u.Multi("Apps", opts, []bool{true, true, false})
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%q: %v %v, want %v", keys, got, err, want)
		}
		if !strings.Contains(out.String(), "✓") {
			t.Errorf("%q: not collapsed", keys)
		}
	}
}

func TestInput(t *testing.T) {
	check := func(v string) string {
		if v == "taken" {
			return "an app has that name"
		}
		return ""
	}
	for keys, want := range map[string]string{"\r": "shop", "eu\r": "eu", "eux\x7f\r": "eu", "taken\rbits\x15mine\r": "mine", "a b\r": "a b"} {
		u, out, _ := ui(keys)
		got, err := u.Input("Name it", "shop", check)
		if err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", keys, got, err, want)
		}
		if strings.HasPrefix(keys, "taken") && !strings.Contains(out.String(), "an app has that name") {
			t.Errorf("the refusal wasn't shown: %q", out.String())
		}
	}
}

func TestStops(t *testing.T) {
	for keys, want := range map[string]error{"\x03": ErrInterrupted, "": ErrClosed, "zzz": ErrClosed} {
		u, _, restored := ui(keys)
		if _, err := u.Confirm("q", true); !errors.Is(err, want) || *restored != 1 {
			t.Errorf("Confirm %q: %v, restored %d", keys, err, *restored)
		}
		u, _, _ = ui(keys)
		if _, err := u.Select("q", []Option{{"a", ""}}, 0); !errors.Is(err, want) {
			t.Errorf("Select %q: %v", keys, err)
		}
		u, _, _ = ui(keys)
		if _, err := u.Multi("q", []Option{{"a", ""}}, []bool{true}); !errors.Is(err, want) {
			t.Errorf("Multi %q: %v", keys, err)
		}
		u, _, _ = ui(strings.TrimSuffix(keys, "zzz"))
		if _, err := u.Input("q", "d", nil); !errors.Is(err, want) {
			t.Errorf("Input %q: %v", keys, err)
		}
		u, _, _ = ui(keys)
		if _, err := u.Review("Review", nil); !errors.Is(err, want) {
			t.Errorf("Review %q: %v", keys, err)
		}
	}
}

func TestReview(t *testing.T) {
	for keys, want := range map[string]Action{"\r": Go, "e": Edit, "q": Quit, "\x1b": Quit, "z\r": Go} {
		u, out, _ := ui(keys)
		got, err := u.Review("Review", [][2]string{{"target", "dev"}, {"apps", "dashboard"}})
		if err != nil || got != want || !strings.Contains(out.String(), "dashboard") {
			t.Errorf("%q: %v %v", keys, got, err)
		}
	}
}

func TestSpin(t *testing.T) {
	u, out, _ := ui("")
	u.Spin("Starting containers")(true)
	u.Spin("Running migrations")(false)
	got := stripped(out.String())
	if !strings.Contains(got, "✓ Starting containers\n") || !strings.Contains(got, "✗ Running migrations\n") {
		t.Errorf("spinner ends: %q", got)
	}
}

func TestFit(t *testing.T) {
	line := bold + "abcdefghijklmnopqrstuvwxyz" + reset
	if got := fit(line, 12); stripped(got) != "abcdefghij…" {
		t.Errorf("fit = %q", stripped(got))
	}
	if got := fit(line, 80); got != line {
		t.Errorf("a short line changed: %q", got)
	}
}

func TestNoteWraps(t *testing.T) {
	u, out, _ := ui("")
	u.width = 30
	u.Section("Apps", "The Go backend always runs. The", "frontends are optional - skip whichever you like.")
	lines := strings.Split(strings.TrimSpace(stripped(out.String())), "\n")
	if len(lines) < 3 || !strings.Contains(out.String(), "The frontends") {
		t.Errorf("not one wrapped paragraph: %q", lines)
	}
	for _, l := range lines {
		if len([]rune(l)) > 30 {
			t.Errorf("wider than the terminal: %q", l)
		}
	}
}

func TestReviewKeepsTheAnswersWithoutItsKeys(t *testing.T) {
	u, out, _ := ui("\r")
	if _, err := u.Review("Review", [][2]string{{"target", "dev"}}); err != nil {
		t.Fatal(err)
	}
	last := out.String()[strings.LastIndex(out.String(), "\x1b[J"):]
	if strings.Contains(last, "go ahead") || !strings.Contains(last, "target") {
		t.Errorf("the review as left: %q", stripped(last))
	}
}
