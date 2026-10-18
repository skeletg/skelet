package experiment

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service is experiment CRUD + the storefront-facing active list + results
type Service struct{ repo Repository }

// NewService wires the experiment service
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Input is the admin-mutable experiment shape
type Input struct {
	ChannelID   uuid.UUID
	Key         string
	Name        string
	Status      string
	Coverage    int
	GoalEvent   string
	SegmentSlug string
	Variations  []Variation
}

var (
	ErrNotFound = errors.New("experiment not found")
	ErrInvalid  = errors.New("invalid experiment")
	ErrKeyTaken = errors.New("another experiment in this channel already uses that key")
	// A visitor's bucket comes from the variations, weights, coverage and targeting, and the goal is what the results count - change
	// any of them mid-run and visitors re-bucket while the results describe a test that no longer exists
	ErrLocked = errors.New("a started experiment's variations, weights, coverage, goal and targeting are fixed - stop it and start a new one")
)

// Every visitor gets the running list, publicly cached, so its size is bounded
const (
	maxVariations     = 10
	maxVariationValue = 2 << 10
)

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func validate(in Input) error {
	if !keyRe.MatchString(in.Key) {
		return invalid("key %q must be lowercase letters, digits, dashes or underscores, up to 64", in.Key)
	}
	if strings.TrimSpace(in.Name) == "" {
		return invalid("name is required")
	}
	switch in.Status {
	case StatusDraft, StatusRunning, StatusStopped:
	default:
		return invalid("unknown status %q", in.Status)
	}
	if in.Coverage < 0 || in.Coverage > 100 {
		return invalid("coverage must be between 0 and 100")
	}
	if len(in.Variations) < 2 {
		return invalid("an experiment needs at least 2 variations")
	}
	if len(in.Variations) > maxVariations {
		return invalid("an experiment has at most %d variations", maxVariations)
	}
	seen := map[string]bool{}
	for _, v := range in.Variations {
		if strings.TrimSpace(v.Key) == "" {
			return invalid("each variation needs a key")
		}
		if seen[v.Key] {
			return invalid("duplicate variation key %q", v.Key)
		}
		seen[v.Key] = true
		if v.Weight < 0 {
			return invalid("variation %q weight cannot be negative", v.Key)
		}
		if len(v.Value) > maxVariationValue {
			return invalid("variation %q value is over %d KB", v.Key, maxVariationValue>>10)
		}
	}
	return nil
}

// What a started experiment may still change: its name and its status
func locked(was Experiment, in Input) bool {
	return was.Status != StatusDraft && (!sameArms(was.Variations, in.Variations) || was.Coverage != in.Coverage ||
		was.GoalEvent != in.GoalEvent || was.SegmentSlug != in.SegmentSlug)
}

// A variation's display name is free to change - only what buckets a visitor or renders for them counts
func sameArms(a, b []Variation) bool {
	return slices.EqualFunc(a, b, func(x, y Variation) bool { return x.Key == y.Key && x.Weight == y.Weight && x.Value == y.Value })
}

func (s *Service) Create(ctx context.Context, in Input) (Experiment, error) {
	if err := validate(in); err != nil {
		return Experiment{}, err
	}
	return s.repo.Create(ctx, Experiment{
		ChannelID: in.ChannelID, Key: in.Key, Name: in.Name, Status: in.Status,
		Coverage: in.Coverage, GoalEvent: in.GoalEvent, SegmentSlug: in.SegmentSlug, Variations: in.Variations,
	})
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (Experiment, error) {
	was, found, err := s.repo.Get(ctx, id)
	if err != nil {
		return Experiment{}, err
	}
	if !found {
		return Experiment{}, ErrNotFound
	}
	in.Key = was.Key // the key names the experiment's exposure events, so it never changes
	if err := validate(in); err != nil {
		return Experiment{}, err
	}
	if locked(was, in) {
		return Experiment{}, ErrLocked
	}
	ok, err := s.repo.Update(ctx, Experiment{
		ID: id, ChannelID: in.ChannelID, Name: in.Name, Status: in.Status, Coverage: in.Coverage,
		GoalEvent: in.GoalEvent, SegmentSlug: in.SegmentSlug, Variations: in.Variations,
	})
	if err != nil {
		return Experiment{}, err
	}
	if !ok {
		return Experiment{}, ErrNotFound
	}
	e, _, err := s.repo.Get(ctx, id)
	return e, err
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Experiment, bool, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, channelID uuid.UUID) ([]Experiment, error) {
	return s.repo.List(ctx, channelID)
}

// Active returns running experiments - the storefront SDK's config payload
func (s *Service) Active(ctx context.Context, channelID uuid.UUID) ([]Experiment, error) {
	return s.repo.ListRunning(ctx, channelID)
}

// Delete and Update match on the channel as well as the id, so a row of another channel is never touched
func (s *Service) Delete(ctx context.Context, channelID, id uuid.UUID) error {
	return s.repo.Delete(ctx, channelID, id)
}

// Results returns the analysed experiment results over the window: per-variation exposed/converted with a Wilson CI,
// uplift + chance-to-win vs the control, a significance-gated winner, and a sample-ratio-mismatch health check
func (s *Service) Results(ctx context.Context, id uuid.UUID, since time.Time) (Experiment, Analysis, error) {
	e, ok, err := s.repo.Get(ctx, id)
	if err != nil {
		return Experiment{}, Analysis{}, err
	}
	if !ok {
		return Experiment{}, Analysis{}, ErrNotFound
	}
	res, err := s.repo.Results(ctx, e.ChannelID, e.Key, e.GoalEvent, since)
	if err != nil {
		return e, Analysis{}, err
	}
	raw := make(map[string]VariationResult, len(res))
	for _, v := range res {
		raw[v.Variation] = v
	}
	return e, Analyze(e.Variations, raw), nil
}

// GoalEvents returns the custom-event names the storefront has fired for the
// channel - goal-event suggestions for the editor (most-frequent first)
func (s *Service) GoalEvents(ctx context.Context, channelID uuid.UUID, since time.Time) ([]string, error) {
	return s.repo.GoalEvents(ctx, channelID, since)
}

// Summary is one experiment's headline result for the Prism overview hub
type Summary struct {
	Experiment          Experiment
	TotalExposed        int
	Conversions         int     // total conversions across all arms
	Leader              string  // leading variation name (highest rate w/ data); "" if no data
	LeaderRate          float64 // the leader's conversion rate
	LeaderUplift        float64 // the leader's relative uplift vs control (0 if leader is control)
	LeaderChanceToWin   float64 // 0 when the leader is the control / no data
	LeaderIsControl     bool
	Winner              string // significance-gated winner name; "" if none
	SampleRatioMismatch bool
}

// Overview is a channel's whole-A/B-program snapshot for the /prism hub: status counts plus a per-running-experiment
// summary (leader + verdict), most-exposed first
type Overview struct {
	Running      int
	Draft        int
	Stopped      int
	TotalExposed int
	Decided      int // running experiments that have a significant winner
	Experiments  []Summary
}

// Overview aggregates the channel's experiments: it tallies statuses and, for each running experiment, runs the same
// Analyze used by Results so the hub and the detail page agree on the leader and the winner
func (s *Service) Overview(ctx context.Context, channelID uuid.UUID, since time.Time) (Overview, error) {
	exps, err := s.repo.List(ctx, channelID)
	if err != nil {
		return Overview{}, err
	}
	var ov Overview
	for _, e := range exps {
		switch e.Status {
		case StatusRunning:
			ov.Running++
		case StatusDraft:
			ov.Draft++
		case StatusStopped:
			ov.Stopped++
		}
		if e.Status != StatusRunning {
			continue
		}
		res, err := s.repo.Results(ctx, e.ChannelID, e.Key, e.GoalEvent, since)
		if err != nil {
			return Overview{}, err
		}
		raw := make(map[string]VariationResult, len(res))
		for _, v := range res {
			raw[v.Variation] = v
		}
		a := Analyze(e.Variations, raw)
		sum := Summary{Experiment: e, TotalExposed: a.TotalExposed, SampleRatioMismatch: a.SampleRatioMismatch}
		bestRate := -1.0
		for _, v := range a.Variations {
			sum.Conversions += v.Converted
			if v.Exposed > 0 && v.Rate > bestRate {
				bestRate = v.Rate
				sum.Leader, sum.LeaderChanceToWin, sum.LeaderIsControl = v.Name, v.ChanceToWin, v.IsControl
				sum.LeaderRate, sum.LeaderUplift = v.Rate, v.Uplift
			}
			if v.Variation == a.Winner {
				sum.Winner = v.Name
			}
		}
		if a.Winner != "" {
			ov.Decided++
		}
		ov.TotalExposed += a.TotalExposed
		ov.Experiments = append(ov.Experiments, sum)
	}
	sort.SliceStable(ov.Experiments, func(i, j int) bool {
		return ov.Experiments[i].TotalExposed > ov.Experiments[j].TotalExposed
	})
	return ov, nil
}
