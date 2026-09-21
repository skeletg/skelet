package plugin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Plugins register named probes via sdk.RegisterHealthCheck / RegisterReadinessCheck, recorded under the calling plugin's slug plus the supplied name
// Probes run concurrently with a per-probe timeout, so total response time is bounded by the slowest single probe rather than their sum, and a slow downstream reports "timeout" instead of blocking /healthz

// One short context per probe, so a runaway plugin can't hold the response open while an operator waits for the page to load
const probeTimeout = 2 * time.Second

type probe struct {
	pluginSlug string
	name       string
	check      func(ctx context.Context) error
}

type healthProbes struct {
	mu        sync.Mutex // reads happen at request time, long after every init(), so contention is essentially zero
	health    []probe
	readiness []probe
}

// Package-level rather than on *Host, because the handlers have no host wiring threaded through and going via installedHost would grab the host pointer on every request
var healthRegistry = &healthProbes{}

// Name is qualified by the plugin's manifest slug, appearing as "<slug>.<name>" - it falls back to just <name> for core-side callers that pass no slug
func (h *Host) RegisterHealthCheck(slug, name string, check sdk.HealthCheck) error {
	healthRegistry.mu.Lock()
	defer healthRegistry.mu.Unlock()
	healthRegistry.health = append(healthRegistry.health, probe{
		pluginSlug: slug,
		name:       name,
		check:      check,
	})
	return nil
}

// Reads any registered config path at the default scope - exposed through sdk.PluginConfig for probes that have no request-scoped ConfigReader
// Returns ("", nil) before SetConfigService has wired configSvc; probes re-read on every /healthz call, so the value resolves later in boot
func (h *Host) ReadConfigString(ctx context.Context, path string) (string, error) {
	svc := h.configSvc.Load()
	if svc == nil {
		return "", nil
	}
	return svc.GetString(ctx, path, config.DefaultScope)
}

func (h *Host) ReadConfigBool(ctx context.Context, path string) (bool, error) {
	svc := h.configSvc.Load()
	if svc == nil {
		return false, nil
	}
	return svc.GetBool(ctx, path, config.DefaultScope)
}

func (h *Host) ReadConfigInt(ctx context.Context, path string) (int64, error) {
	svc := h.configSvc.Load()
	if svc == nil {
		return 0, nil
	}
	return svc.GetInt(ctx, path, config.DefaultScope)
}

func (h *Host) RegisterReadinessCheck(slug, name string, check sdk.ReadinessCheck) error {
	healthRegistry.mu.Lock()
	defer healthRegistry.mu.Unlock()
	healthRegistry.readiness = append(healthRegistry.readiness, probe{
		pluginSlug: slug,
		name:       name,
		check:      check,
	})
	return nil
}

type probeResult struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type healthResponse struct {
	OK     bool          `json:"ok"`
	Checks []probeResult `json:"checks"`
}

// Aggregates every plugin-registered health probe, each capped at probeTimeout - always 200, with per-probe state in the body
// Mount at /healthz from cmd/server/main.go
func HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthRegistry.mu.Lock()
		probes := append([]probe(nil), healthRegistry.health...)
		healthRegistry.mu.Unlock()
		writeProbeResults(w, r, probes)
	})
}

// The readiness counterpart to HealthHandler - mount at /readyz
func ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthRegistry.mu.Lock()
		probes := append([]probe(nil), healthRegistry.readiness...)
		healthRegistry.mu.Unlock()
		writeProbeResults(w, r, probes)
	})
}

func writeProbeResults(w http.ResponseWriter, r *http.Request, probes []probe) {
	results := runProbes(r.Context(), enabledProbes(r.Context(), probes))
	allOK := true
	for _, res := range results {
		if !res.OK {
			allOK = false
			break
		}
	}
	// Always 200, even when a probe fails - these endpoints REPORT plugin state, they don't gate on it, and monitoring should alert on ok in the body
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{OK: allOK, Checks: results})
}

// A disabled plugin is not part of the health picture - leaving its probe in means a plugin an operator turned off can still 503 /readyz
// and pull the pod from rotation Default scope, since a probe has no channel context, matching how event handlers gate
func enabledProbes(ctx context.Context, probes []probe) []probe {
	out := probes[:0:0]
	for _, p := range probes {
		if p.pluginSlug != "" && !isEnabledFromContext(ctx, p.pluginSlug, uuid.Nil) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Sorted by name so output stays stable across requests
func runProbes(ctx context.Context, probes []probe) []probeResult {
	if len(probes) == 0 {
		return nil
	}
	results := make([]probeResult, len(probes))
	var wg sync.WaitGroup
	wg.Add(len(probes))
	for i, p := range probes {
		go func() {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			err := safeCheck(probeCtx, p.check)
			name := qualifiedName(p.pluginSlug, p.name)
			results[i] = probeResult{
				Name: name,
				OK:   err == nil,
			}
			if err != nil {
				// These endpoints are unauthenticated - a gateway probe error carries hostnames, key prefixes and upstream messages, so the detail goes to the log and the body says which probe failed, nothing more
				slog.Warn("plugin health probe failed", "probe", name, "plugin", p.pluginSlug, "err", err)
				results[i].Error = "probe failed - see server logs"
			}
		}()
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

// A recover per probe, so a panicking one fails locally rather than taking the /healthz response down
func safeCheck(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = errFromPanic(rec)
		}
	}()
	return fn(ctx)
}

func qualifiedName(slug, name string) string {
	if slug == "" {
		return name
	}
	return slug + "." + name
}

// ProbeStatus is one probe's outcome for the admin health panel. The HTTP handlers above answer orchestrators; this
// answers an operator reading a page, so it keeps the plugin slug separate rather than pre-joining it into a name
type ProbeStatus struct {
	Plugin string
	Name   string
	OK     bool
	Error  string
}

// HealthStatuses and ReadinessStatuses run the same probes /healthz and /readyz do,
// including the disabled-plugin filter, and hand back structured results
func HealthStatuses(ctx context.Context) []ProbeStatus {
	healthRegistry.mu.Lock()
	probes := append([]probe(nil), healthRegistry.health...)
	healthRegistry.mu.Unlock()
	return statuses(ctx, probes)
}

func ReadinessStatuses(ctx context.Context) []ProbeStatus {
	healthRegistry.mu.Lock()
	probes := append([]probe(nil), healthRegistry.readiness...)
	healthRegistry.mu.Unlock()
	return statuses(ctx, probes)
}

func statuses(ctx context.Context, probes []probe) []ProbeStatus {
	enabled := enabledProbes(ctx, probes)
	bySlug := make(map[string]string, len(enabled))
	for _, p := range enabled {
		bySlug[qualifiedName(p.pluginSlug, p.name)] = p.pluginSlug
	}
	results := runProbes(ctx, enabled)
	out := make([]ProbeStatus, 0, len(results))
	for _, r := range results {
		out = append(out, ProbeStatus{Plugin: bySlug[r.Name], Name: r.Name, OK: r.OK, Error: r.Error})
	}
	return out
}
