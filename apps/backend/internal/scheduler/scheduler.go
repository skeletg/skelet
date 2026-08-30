// Package scheduler runs recurring jobs whose schedule is admin-configurable (a cron expression or an "@every" interval) while the work is either a code-registered handler or an admin-defined webhook call
// One engine ticks on a fixed poll interval, atomically claims due rows (FOR UPDATE SKIP LOCKED plus a short lease), runs them with panic recovery and a timeout, then records the outcome and computes the next run - the atomic claim makes it safe on every replica with no leader election
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

const (
	defaultPollInterval = 30 * time.Second
	// The per-job run timeout when a code job doesn't set one - overridable via
	// RegisterWithTimeout or the admin, stored in scheduled_jobs.timeout_seconds. The
	// claim lease is a job's own timeout + a grace, computed per row in the claim SQL, so
	// a long-running job can't be re-claimed by another replica mid-flight
	defaultRunTimeout = 5 * time.Minute
	runWorkers        = 4
	maxBackoff        = 6 * time.Hour
	// Upper bound on an admin-set timeout - the claim lease is timeout+grace, so an
	// absurd value would leave a crashed run leased (and the job dead) for that long
	maxRunTimeoutSeconds = 24 * 60 * 60
)

// A code-registered job body - it should honor ctx, cancelled on shutdown or run timeout, and return an error to record a failed run
type HandlerFunc func(ctx context.Context) error

// Marks a code job whose handler isn't registered, e.g. the plugin that registered it was removed - such a job is parked with next_run cleared rather than erroring every tick forever
var errNoHandler = errors.New("no handler registered")

type registered struct {
	enabled         bool
	defaultSchedule string
	description     string
	timeout         time.Duration
	handler         HandlerFunc
}

// Accepts standard 5-field cron expressions AND descriptors like "@every 15m" or "@daily", covering both cases from one grammar
var scheduleParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// Exported so the admin resolver can reject a bad cron or @every before saving
func ParseSchedule(spec string) (cron.Schedule, error) {
	return scheduleParser.Parse(spec)
}

func NextRun(spec string, from time.Time) (time.Time, bool) {
	return NextRunInTZ(spec, "", from)
}

// For cron expressions it prepends "CRON_TZ=<tz>" so robfig evaluates field matches in that zone - its Schedule.Next converts internally, so `from` is NOT pre-converted - The zone is irrelevant for @every and descriptor schedules
// Empty tz means server time, and it returns false on an invalid schedule or an unknown zone
func NextRunInTZ(spec, tz string, from time.Time) (time.Time, bool) {
	sched, err := parseInTZ(spec, tz)
	if err != nil {
		return time.Time{}, false
	}
	return sched.Next(from), true
}

// Returns a readable error for the admin, distinguishing a bad cron from a bad zone
func ValidateScheduleTZ(spec, tz string) error {
	_, err := parseInTZ(spec, tz)
	return err
}

func parseInTZ(spec, tz string) (cron.Schedule, error) {
	spec = strings.TrimSpace(spec)
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		// @descriptors ignore CRON_TZ, so only prefix real cron expressions
		if !strings.HasPrefix(spec, "@") {
			spec = "CRON_TZ=" + tz + " " + spec
		}
	}
	return scheduleParser.Parse(spec)
}

// The next `count` fire times for the admin's "next runs" preview - empty when the schedule is invalid
func PreviewRuns(spec, tz string, count int) []time.Time {
	if count <= 0 {
		count = 5
	}
	sched, err := parseInTZ(spec, tz)
	if err != nil {
		return nil
	}
	out := make([]time.Time, 0, count)
	t := time.Now()
	for i := 0; i < count; i++ {
		t = sched.Next(t)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out
}

// Engine-global settings, the pause switch and alert sink - optional, and a nil config means enabled with no alerting
type Config interface {
	SchedulerEnabled(ctx context.Context) bool
	AlertWebhookURL(ctx context.Context) string
}

type Service struct {
	queries      *db.Queries
	pollInterval time.Duration
	webhook      *webhookRunner
	cfg          Config

	// Bounds concurrent runs across ticks; runWG lets Run drain in-flight jobs on shutdown
	sem   chan struct{}
	runWG sync.WaitGroup

	mu       sync.RWMutex
	handlers map[string]registered
}

// allowPrivateWebhookHosts loosens the webhook SSRF guard so dev jobs can hit localhost and private IPs - keep it false in prod
func New(queries *db.Queries, allowPrivateWebhookHosts bool) *Service {
	return &Service{
		queries:      queries,
		pollInterval: defaultPollInterval,
		webhook:      newWebhookRunner(allowPrivateWebhookHosts),
		sem:          make(chan struct{}, runWorkers),
		handlers:     map[string]registered{},
	}
}

func (s *Service) SetConfig(c Config) { s.cfg = c }

func (s *Service) SetPollInterval(d time.Duration) {
	if d > 0 {
		s.pollInterval = d
	}
}

// Call before Seed and Run - defaultSchedule seeds the DB row on first boot, and an operator's later edits win
func (s *Service) Register(name, defaultSchedule, description string, handler HandlerFunc) {
	s.register(name, defaultSchedule, description, defaultRunTimeout, true, handler)
}

// RegisterDisabled seeds the row switched off, for work nobody should be doing until they ask for it - an operator
// enabling it in the UI sticks, since the seed only sets the default on first insert
func (s *Service) RegisterDisabled(name, defaultSchedule, description string, handler HandlerFunc) {
	s.register(name, defaultSchedule, description, defaultRunTimeout, false, handler)
}

// RegisterWithTimeout is Register with an explicit per-job run timeout, for jobs that
// legitimately run longer than the 5-minute default - a full backup, a large reindex
func (s *Service) RegisterWithTimeout(name, defaultSchedule, description string, timeout time.Duration, handler HandlerFunc) {
	s.register(name, defaultSchedule, description, timeout, true, handler)
}

func (s *Service) register(name, defaultSchedule, description string, timeout time.Duration, enabled bool, handler HandlerFunc) {
	if _, err := ParseSchedule(defaultSchedule); err != nil {
		// A bad default is a programming error - fail loud at boot
		panic(fmt.Sprintf("scheduler: invalid default schedule %q for job %q: %v", defaultSchedule, name, err))
	}
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[name] = registered{enabled: enabled, defaultSchedule: defaultSchedule, description: description, timeout: timeout, handler: handler}
}

// Upserts a row per registered code job (ON CONFLICT DO NOTHING) and stamps the initial next_run_at - call once, after every Register
func (s *Service) Seed(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	for name, r := range s.handlers {
		next, _ := NextRun(r.defaultSchedule, now)
		if err := s.queries.RegisterScheduledJob(ctx, db.RegisterScheduledJobParams{
			Name:           name,
			Schedule:       r.defaultSchedule,
			Description:    r.description,
			NextRunAt:      sql.NullTime{Time: next, Valid: true},
			TimeoutSeconds: int32(r.timeout.Seconds()),
			Enabled:        r.enabled,
		}); err != nil {
			return fmt.Errorf("seed job %q: %w", name, err)
		}
	}
	return nil
}

func (s *Service) Run(ctx context.Context) {
	// Drain in-flight runs before returning so shutdown still waits on them
	defer s.runWG.Wait()
	t := time.NewTicker(s.pollInterval)
	defer t.Stop()
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// Claims at most as many rows as there are free worker slots and returns WITHOUT waiting for the runs - a 30-minute backup keeps its one
// slot while everything else claims and runs on subsequent polls; waiting for the whole batch would freeze every job behind it
func (s *Service) tick(ctx context.Context) {
	// Global pause switch - skip claiming entirely while disabled, so due jobs just wait with next_run_at in the past and fire once re-enabled
	if s.cfg != nil && !s.cfg.SchedulerEnabled(ctx) {
		return
	}
	for ctx.Err() == nil {
		// Only tick acquires slots, so this count can only be pessimistic - never over-claims
		free := runWorkers - len(s.sem)
		if free <= 0 {
			return
		}
		rows, err := s.queries.ClaimDueScheduledJobs(ctx, db.ClaimDueScheduledJobsParams{
			Now:       time.Now(),
			BatchSize: int32(free),
		})
		if err != nil {
			slog.Error("scheduler: claim due jobs", "err", err)
			return
		}
		for _, row := range rows {
			s.sem <- struct{}{}
			s.runWG.Go(func() {
				defer func() { <-s.sem }()
				s.runOne(ctx, row)
			})
		}
		if len(rows) < free {
			return
		}
	}
}

// Success resets attempt and failure state and schedules the next cron occurrence; a failure with attempts left retries on exponential backoff; exhausting them resumes the normal cadence
// Consecutive failures fire a single alert at the configured threshold, and the outcome is always recorded (even on panic) so the lease is released, with one run-history row per run
func (s *Service) runOne(ctx context.Context, row db.ScheduledJob) {
	// invoke recovers the HANDLER's panics - this covers runOne's own bookkeeping (cron/tz parsing,
	// alert marshalling, outcome recording), which would otherwise panic straight through the worker goroutine
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("scheduled job bookkeeping panicked", "job", row.Name, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
		}
	}()
	start := time.Now()
	attempt := row.CurrentAttempt + 1 // 1-based attempt of the current occurrence

	// Derived from ctx so a shutdown signal cancels the in-flight run promptly, with the job's own timeout still bounding a wedged handler
	// This used WithoutCancel before, so a long job ignored shutdown and pinned bg.Wait()'s grace period until the pool closed
	timeout := time.Duration(row.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	runErr := s.invoke(runCtx, row)
	cancel()
	finished := time.Now()
	durMs := int32(finished.Sub(start).Milliseconds())

	// Interrupted by shutdown rather than a genuine failure - release the lease at the normal cadence without touching retry or alert state, so it re-claims cleanly on the next boot and a deploy can't trip a failure alert
	// Uses a detached context, since ctx is already done
	if runErr != nil && ctx.Err() != nil {
		rec, cancelRec := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancelRec()
		if err := s.queries.RecordScheduledJobResult(rec, db.RecordScheduledJobResultParams{
			ID:                  row.ID,
			LastStatus:          "error",
			LastError:           "interrupted: server shutting down",
			LastDurationMs:      sql.NullInt32{Int32: durMs, Valid: true},
			RanAt:               sql.NullTime{Time: start, Valid: true},
			NextRunAt:           nextRunFrom(row, finished),
			CurrentAttempt:      row.CurrentAttempt,
			ConsecutiveFailures: row.ConsecutiveFailures,
			Alerted:             row.Alerted,
		}); err != nil {
			slog.Error("scheduler: record interrupted result", "job", row.Name, "err", err)
		}
		slog.Info("scheduler: job interrupted by shutdown", "job", row.Name)
		return
	}

	// Decide the state transition (pure), then apply its side effects
	dec := decideOutcome(row, attempt, runErr)
	status := dec.status
	nextAttempt, consecFail, alerted := dec.nextAttempt, dec.consecFail, dec.alerted
	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}

	var next sql.NullTime
	switch {
	case dec.parked:
		// Parked - a code job whose handler is gone, so next_run stays null and it stops being claimed - One history row records the park
		slog.Warn("scheduler: parking job with no handler", "job", row.Name)
	case dec.retrySoon:
		// Retry soon on exponential backoff - re-claimed normally once next_run passes
		next = sql.NullTime{Time: finished.Add(backoffDuration(row.RetryBackoffSeconds, attempt)), Valid: true}
		slog.Error("scheduler: job failed", "job", row.Name, "kind", row.Kind, "attempt", attempt, "consecutive_failures", consecFail, "err", runErr)
	case status == "error":
		next = nextRunFrom(row, finished)
		slog.Error("scheduler: job failed", "job", row.Name, "kind", row.Kind, "attempt", attempt, "consecutive_failures", consecFail, "err", runErr)
	default:
		next = nextRunFrom(row, finished)
		slog.Info("scheduler: job ran", "job", row.Name, "kind", row.Kind, "attempt", attempt, "ms", durMs)
	}
	// Fire a single alert when consecutive failures first cross the threshold
	if dec.shouldAlert {
		s.fireAlert(ctx, row, consecFail, errMsg)
	}

	// Record outcome and state on a fresh context, since runCtx may be cancelled
	rec, cancelRec := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelRec()
	if err := s.queries.RecordScheduledJobResult(rec, db.RecordScheduledJobResultParams{
		ID:                  row.ID,
		LastStatus:          status,
		LastError:           truncate(errMsg, 2000),
		LastDurationMs:      sql.NullInt32{Int32: durMs, Valid: true},
		RanAt:               sql.NullTime{Time: start, Valid: true},
		NextRunAt:           next,
		CurrentAttempt:      nextAttempt,
		ConsecutiveFailures: consecFail,
		Alerted:             alerted,
	}); err != nil {
		slog.Error("scheduler: record result", "job", row.Name, "err", err)
	}

	// A separate statement - losing one run-history row is acceptable and keeps it off the record hot path
	if err := s.queries.InsertScheduledJobRun(rec, db.InsertScheduledJobRunParams{
		JobID:      row.ID,
		JobName:    row.Name,
		StartedAt:  start,
		FinishedAt: finished,
		Status:     status,
		Error:      truncate(errMsg, 2000),
		DurationMs: durMs,
		Attempt:    attempt,
	}); err != nil {
		slog.Error("scheduler: record run history", "job", row.Name, "err", err)
	}
}

// The pure post-run state transition from decideOutcome - runOne turns it into DB writes plus the next_run time, which needs the clock
type runDecision struct {
	status      string // "success" | "error"
	nextAttempt int32  // current_attempt to persist (0 = idle)
	consecFail  int32  // consecutive_failures to persist
	alerted     bool   // alerted latch to persist
	shouldAlert bool   // fire the alert webhook this run
	retrySoon   bool   // schedule at finished+backoff (else cron cadence)
	parked      bool   // no handler → leave next_run null
}

// The retry and alert state machine, pure with no DB or clock so the decision table is unit-testable - `attempt` is the 1-based attempt number of the current occurrence
func decideOutcome(row db.ScheduledJob, attempt int32, runErr error) runDecision {
	d := runDecision{
		status:      "success",
		nextAttempt: row.CurrentAttempt,
		consecFail:  row.ConsecutiveFailures,
		alerted:     row.Alerted,
	}
	switch {
	case errors.Is(runErr, errNoHandler):
		// Parked - don't touch retry or alert counters
		d.status = "error"
		d.parked = true
	case runErr == nil:
		d.status = "success"
		d.nextAttempt, d.consecFail, d.alerted = 0, 0, false
	default:
		d.status = "error"
		d.consecFail = row.ConsecutiveFailures + 1
		if attempt < row.MaxAttempts {
			d.nextAttempt = attempt
			d.retrySoon = true
		} else {
			d.nextAttempt = 0 // exhausted → resume cadence
		}
		if row.AlertAfterFailures > 0 && d.consecFail >= row.AlertAfterFailures && !row.Alerted {
			d.shouldAlert = true
			d.alerted = true
		}
	}
	return d
}

// The next cron occurrence in the job's timezone - a null time parks the job when the schedule or zone is invalid
func nextRunFrom(row db.ScheduledJob, from time.Time) sql.NullTime {
	if t, ok := NextRunInTZ(row.Schedule, row.Timezone, from); ok {
		return sql.NullTime{Time: t, Valid: true}
	}
	return sql.NullTime{}
}

// base * 2^(attempt-1), capped at maxBackoff
func backoffDuration(baseSeconds, attempt int32) time.Duration {
	if baseSeconds <= 0 {
		baseSeconds = 60
	}
	shift := min(attempt-1,
		// guard the shift; 2^16 already dwarfs the cap
		16)
	d := time.Duration(baseSeconds) * time.Second * time.Duration(int64(1)<<uint(shift))
	if d > maxBackoff || d <= 0 {
		return maxBackoff
	}
	return d
}

// POSTs to the global alert webhook if configured, reusing the SSRF-guarded webhook runner - best-effort, since the caller still latches `alerted` so a flapping endpoint can't spam
func (s *Service) fireAlert(ctx context.Context, row db.ScheduledJob, failures int32, lastErr string) {
	if s.cfg == nil {
		return
	}
	url := strings.TrimSpace(s.cfg.AlertWebhookURL(ctx))
	if url == "" {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"job":                  row.Name,
		"kind":                 row.Kind,
		"consecutive_failures": failures,
		"last_error":           truncate(lastErr, 1000),
		"at":                   time.Now().UTC().Format(time.RFC3339),
	})
	payload, _ := json.Marshal(WebhookConfig{URL: url, Method: "POST", Body: string(body)})
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.webhook.run(actx, payload); err != nil {
		slog.Error("scheduler: alert webhook failed", "job", row.Name, "err", err)
	}
}

// Dispatches to the code handler or the webhook runner, recovering panics
func (s *Service) invoke(ctx context.Context, row db.ScheduledJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			slog.Error("scheduler: job panicked", "job", row.Name, "panic", r, "stack", string(debug.Stack()))
		}
	}()

	switch row.Kind {
	case "webhook":
		return s.webhook.run(ctx, row.Payload)
	case "code":
		s.mu.RLock()
		reg, ok := s.handlers[row.Name]
		s.mu.RUnlock()
		if !ok {
			return fmt.Errorf("%w: code job %q (registering code removed?)", errNoHandler, row.Name)
		}
		return reg.handler(ctx)
	default:
		return fmt.Errorf("unknown job kind %q", row.Kind)
	}
}

// Pulls a job's next run forward so the engine executes it on the next tick - returns the possibly-updated due time
func (s *Service) RunNow(ctx context.Context, id uuid.UUID) error {
	return s.queries.SetScheduledJobNextRun(ctx, db.SetScheduledJobNextRunParams{
		ID:        id,
		NextRunAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
