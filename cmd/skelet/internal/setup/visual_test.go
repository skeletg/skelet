package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
	"github.com/skeletg/skelet/cmd/skelet/internal/tui"
)

const (
	enter = "\r"
	down  = "\x1b[B"
)

func drawn(keys string, out *strings.Builder) prompter {
	raw := func() (func(), error) { return func() {}, nil }
	return prompter{on: true, ui: tui.New(strings.NewReader(keys), out, raw, 120)}
}

func templateCatalog(root string) catalog {
	return catalog{root: root, gateways: []string{"payment-stub"}, storefronts: []storefronts.Storefront{
		{Name: storefronts.DefaultTemplate, Dir: "templates/" + storefronts.DefaultTemplate},
		{Name: "astro-shop", Dir: "templates/astro-shop"},
	}}
}

// apps, shop, template, a refused name then eu-shop, Docker, working tree, target, suites, search, gateways, hardening, tools
var firstRound = enter + enter + enter + storefronts.DefaultTemplate + enter + "\x15eu-shop" + enter + "y" + enter +
	enter + enter + "n" + "2" + enter + enter + " " + enter

func TestDrawnInterview(t *testing.T) {
	root := fixture(t)
	var out strings.Builder
	a, err := answer(root, templateCatalog(root), flags{}, profile.Profile{}, "p", drawn(firstRound, &out), &out)
	want := answers{target: "dev", signals: true, search: "postgres", gateways: []string{"payment-stub"},
		apps: []string{"dashboard", "eu-shop"}, dockerApps: []string{"dashboard", "eu-shop"}, tools: []string{"mailpit"},
		prune: true, removeDeselected: true, shop: "eu-shop", shopTemplate: storefronts.DefaultTemplate}
	if err != nil || fmt.Sprint(a) != fmt.Sprint(want) {
		t.Errorf("answers\n got %+v\nwant %+v\n%s", a, want, out.String())
	}
	for _, shown := range []string{"◆ Apps", "an app or template has that name", "✓", "◆ Dev tools"} {
		if !strings.Contains(out.String(), shown) {
			t.Errorf("never showed %q", shown)
		}
	}
}

func TestReviewGoesRoundAgain(t *testing.T) {
	root := fixture(t)
	again := strings.Repeat(enter, 14)
	var out strings.Builder
	first, _ := answer(root, templateCatalog(root), flags{}, profile.Profile{}, "p", drawn(firstRound, &out), &out)
	a, ok, err := interview(root, templateCatalog(root), flags{}, profile.Profile{}, "p", drawn(firstRound+"e"+again+enter, &out), &out)
	if err != nil || !ok || fmt.Sprint(a) != fmt.Sprint(first) {
		t.Errorf("second round\n got %+v\nwant %+v", a, first)
	}
	if _, ok, _ := interview(root, templateCatalog(root), flags{}, profile.Profile{}, "p", drawn(firstRound+"q", &out), &out); ok {
		t.Error("quitting the review went ahead")
	}
}

func TestInterruptedQuestionAborts(t *testing.T) {
	root := fixture(t)
	t.Cleanup(func() { abort = func(error) {} })
	abort = func(err error) { panic(err) }
	defer func() {
		if err, _ := recover().(error); !errors.Is(err, tui.ErrInterrupted) {
			t.Errorf("recovered %v", err)
		}
	}()
	var out strings.Builder
	_, _ = answer(root, templateCatalog(root), flags{}, profile.Profile{}, "p", drawn("\x03", &out), &out)
	t.Error("Ctrl-C didn't stop the interview")
}

// when it failed
func TestStepper(t *testing.T) {
	root := t.TempDir()
	var out strings.Builder
	ui := tui.New(strings.NewReader(""), &out, nil, 120)
	st, err := newStepper(root, ui)
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	ok := st.run("Starting containers", func() int {
		fmt.Println("compose says hello")
		fmt.Fprintln(os.Stderr, "and warns")
		return 0
	})
	bad := st.run("Running migrations", func() int {
		fmt.Println("migration 42 failed")
		return 3
	})
	st.close()
	if ok != 0 || bad != 3 || os.Stdout != stdout {
		t.Errorf("codes %d %d, stdout restored %v", ok, bad, os.Stdout == stdout)
	}
	log, _ := os.ReadFile(filepath.Join(root, ".skelet", "setup.log"))
	screen := out.String()
	switch {
	case !strings.Contains(string(log), "compose says hello") || !strings.Contains(string(log), "and warns"):
		t.Errorf("log: %q", log)
	case strings.Contains(screen, "compose says hello"):
		t.Error("a stage's output reached the screen")
	case !strings.Contains(screen, "Starting containers\n") || !strings.Contains(screen, "migration 42 failed"):
		t.Errorf("screen: %q", screen)
	}
	plain, _ := newStepper(root, nil)
	if plain.run("x", func() int { return 7 }) != 7 || plain.logPath != "" {
		t.Error("a plain stepper did more than run the stage")
	}
}
