// Command seed fills the dev spine with deterministic demo activity, one shop profile per channel, so every Signals
// report and board has data to show - dev only, and every row it writes is marked so --purge removes exactly those
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/seed"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/spinech"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	seedN := fs.Uint64("seed", 1, "the same seed gives the same data")
	days := fs.Int("days", 90, "how many days back the activity reaches")
	customers := fs.Int("customers", 200, "known customers per shop")
	events := fs.String("events", "", "load set: keep adding visitors until each shop has this many events (500k, 10m)")
	channel := fs.String("channel", "", "one channel id (default: every channel Signals knows)")
	purge := fs.Bool("purge", false, "remove the seeded rows and stop")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if env := os.Getenv("APP_ENV"); env != "development" {
		return fmt.Errorf("refusing to seed with APP_ENV=%q - demo data is for development only", env)
	}
	target, err := count(*events)
	if err != nil {
		return err
	}

	ctx := context.Background()
	st, closeSpine, err := spinech.FromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeSpine()

	channels, err := channelIDs(ctx, *channel)
	if err != nil {
		return err
	}
	// A rerun replaces the earlier seed rather than adding to it - its times move with the clock, so the rows wouldn't collapse
	n, err := st.PurgeSeed(ctx, seed.KeyPrefix, seed.AnonPrefix, channels)
	if err != nil {
		return err
	}
	fmt.Printf("purged %d seeded events\n", n)
	if *purge {
		return nil
	}
	cfg := seed.Config{Seed: *seedN, Now: time.Now().UTC(), Days: *days, Customers: *customers, Events: target}
	for _, ch := range channels {
		n, err := seed.Generate(ch, cfg, &sink{ctx: ctx, st: st})
		if err != nil {
			return fmt.Errorf("channel %s: %w", ch, err)
		}
		fmt.Printf("channel %s: %d events\n", ch, n)
	}
	return st.Flush(ctx)
}

func channelIDs(ctx context.Context, one string) ([]uuid.UUID, error) {
	if one != "" {
		id, err := uuid.Parse(one)
		return []uuid.UUID{id}, err
	}
	db, err := dbconn.Connect(os.Getenv("SIGNALS_DB_URL"), dbconn.WithMaxConns(1))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ids, err := signalsdb.New(db).RosterChannels(ctx)
	if err == nil && len(ids) == 0 {
		err = errors.New("no channels in Signals yet - start the stack so core pushes them, or pass --channel")
	}
	return ids, err
}

// 500k, 10m or a plain number
func count(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	mult := 1
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1_000, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--events %q: want a positive count like 500k or 10m", s)
	}
	return n * mult, nil
}

// The store batches on its own but sheds past its buffer cap, so the seed flushes as it goes
type sink struct {
	ctx context.Context
	st  *spinech.Store
	n   int
}

const flushEvery = 20_000

func (s *sink) Event(e analytics.SpineEvent) error {
	if _, err := s.st.AppendEvent(s.ctx, e); err != nil {
		return err
	}
	if s.n++; s.n%flushEvery == 0 {
		return s.st.Flush(s.ctx)
	}
	return nil
}

func (s *sink) Identify(anon string, cid, ch uuid.UUID) error {
	return s.st.UpsertIdentity(s.ctx, anon, cid, ch)
}
