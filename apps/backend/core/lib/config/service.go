package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

type Service struct {
	repo     Repository
	signals  Repository // the suite's store, when this process can reach it - see storeFor
	cache    *cache.Cache
	cipher   *secrets.Cipher
	onChange []func(ctx context.Context, def Definition)
}

// Runs fn after every successful Set or Reset, with the changed field's definition - register at startup, before the service takes writes
func (s *Service) OnChange(fn func(ctx context.Context, def Definition)) {
	s.onChange = append(s.onChange, fn)
}

func (s *Service) changed(ctx context.Context, def Definition) {
	for _, fn := range s.onChange {
		fn(ctx, def)
	}
}

// Bounds staleness for the unauthenticated publicConfig query - operators tweak settings once a sprint at most, so 10 minutes is comfortable headroom against every storefront page hitting this, and any Set or Reset wipes the entry immediately
const publicConfigCacheTTL = 10 * time.Minute

// Bounds staleness for single-path Get reads - feature toggles, currency base, tax and inventory switches - which sit on hot request paths and would otherwise be an uncached Postgres round-trip each
// Any Set or Reset wipes the whole cache, so the TTL is only a backstop against a missed invalidation
const scalarConfigCacheTTL = 5 * time.Minute

// SetSignalsRepo hands over the suite's settings store. Without it a Signals path resolves to its
// registered default, which is what a core process that cannot reach the suite should answer
func (s *Service) SetSignalsRepo(r Repository) { s.signals = r }

// storeFor picks the store a path's value lives in, which the registry declares on the definition
func (s *Service) storeFor(path string) Repository {
	if def, ok := Find(path); ok && def.Store == StoreSignals {
		return s.signals
	}
	return s.repo
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Optional - nil disables caching, so every ListPublic call hits Postgres
func (s *Service) SetCache(c *cache.Cache) {
	s.cache = c
}

// Encrypts TypeSecret values at rest - optional, and nil disables encryption as a dev convenience, storing secrets as plaintext and reading them back verbatim - Production requires the key, enforced by the boot-time env check
// Only secret-typed paths are affected, and reads decrypt by ciphertext prefix, so toggling the key on doesn't break already-plaintext rows - they pass through until the next write or backfill
func (s *Service) SetCipher(c *secrets.Cipher) {
	s.cipher = c
}

// Encrypted values are prefix-tagged and need a configured cipher; unencrypted plaintext passes through
func (s *Service) decryptStored(v string) (string, error) {
	if !secrets.IsEncrypted(v) {
		return v, nil
	}
	if s.cipher == nil {
		return "", ErrSecretCipherUnavailable
	}
	return s.cipher.Decrypt(v)
}

func (s *Service) invalidate(ctx context.Context) {
	s.cache.Flush(ctx)
}

// Resolution order is the stored row at (scope.Kind, scope.ID), then the stored row at (default, 0), then the Registry default
func (s *Service) Get(ctx context.Context, path string, scope Scope) (Entry, error) {
	def, ok := Find(path)
	if !ok {
		return Entry{}, ErrUnknownPath
	}
	if err := validateScope(scope); err != nil {
		return Entry{}, err
	}

	// Read-through cache - secret-typed paths are never cached, since their resolved value is decrypted plaintext that must not sit in Valkey - A cache miss or outage falls straight through to Postgres, so this only ever reduces load
	cacheable := s.cache != nil && def.Type != TypeSecret
	cacheKey := "get:" + scopeKey(scope, path)
	if cacheable {
		if cached, hit := cache.GetJSON[Entry](ctx, s.cache, cacheKey); hit {
			return cached, nil
		}
	}

	repo := s.storeFor(path)
	if repo == nil {
		return Entry{Definition: def, Value: def.Default, IsDefault: true, Scope: DefaultScope}, nil
	}
	candidates, err := repo.GetCandidates(ctx, scope, path)
	if err != nil {
		if repo == s.signals {
			// Same reason as rowsInScope: a suite that cannot answer reads as the registered default, not as a failed request
			slog.Warn("signals settings unavailable, using the registered default", "path", path, "err", err)
			return Entry{Definition: def, Value: def.Default, IsDefault: true, Scope: DefaultScope}, nil
		}
		return Entry{}, err
	}
	scoped, dflt := pickRow(candidates, scope)

	var entry Entry
	switch {
	case scope != DefaultScope && scoped != nil:
		value, derr := s.decryptStored(scoped.Value)
		if derr != nil {
			return Entry{}, derr
		}
		entry = Entry{
			Definition: def,
			Value:      value,
			IsDefault:  false,
			// The row's own scope, not the requested one - a channel+locale read can resolve to
			// the channel-wide row, and the admin UI reads this to say where a value lives
			Scope:     scoped.Scope,
			UpdatedAt: scoped.UpdatedAt,
		}
	case dflt != nil:
		value, derr := s.decryptStored(dflt.Value)
		if derr != nil {
			return Entry{}, derr
		}
		entry = Entry{
			Definition: def,
			Value:      value,
			IsDefault:  false,
			Scope:      DefaultScope,
			UpdatedAt:  dflt.UpdatedAt,
		}
	default:
		entry = Entry{
			Definition: def,
			Value:      def.Default,
			IsDefault:  true,
			Scope:      DefaultScope,
		}
	}

	if cacheable {
		s.cache.SetJSON(ctx, cacheKey, entry, scalarConfigCacheTTL)
	}
	return entry, nil
}

// Any parse error surfaces as ErrInvalidValue, the same error the admin sees when setting a bad value
func (s *Service) GetString(ctx context.Context, path string, scope Scope) (string, error) {
	e, err := s.Get(ctx, path, scope)
	if err != nil {
		return "", err
	}
	return e.Value, nil
}

func (s *Service) GetInt(ctx context.Context, path string, scope Scope) (int64, error) {
	e, err := s.Get(ctx, path, scope)
	if err != nil {
		return 0, err
	}
	n, parseErr := strconv.ParseInt(e.Value, 10, 64)
	if parseErr != nil {
		return 0, ErrInvalidValue
	}
	return n, nil
}

func (s *Service) GetBool(ctx context.Context, path string, scope Scope) (bool, error) {
	e, err := s.Get(ctx, path, scope)
	if err != nil {
		return false, err
	}
	b, parseErr := strconv.ParseBool(e.Value)
	if parseErr != nil {
		return false, ErrInvalidValue
	}
	return b, nil
}

func (s *Service) GetJSON(ctx context.Context, path string, scope Scope, out any) error {
	e, err := s.Get(ctx, path, scope)
	if err != nil {
		return err
	}
	if jerr := json.Unmarshal([]byte(e.Value), out); jerr != nil {
		return ErrInvalidValue
	}
	return nil
}

// Validates path and scope, normalizes the raw string against the Registry type so stored values are stable under value-equality, then upserts
func (s *Service) Set(ctx context.Context, path, value string, scope Scope) (Entry, error) {
	def, ok := Find(path)
	if !ok {
		return Entry{}, ErrUnknownPath
	}
	if err := validateScope(scope); err != nil {
		return Entry{}, err
	}
	if err := checkScoping(path, scope); err != nil {
		return Entry{}, err
	}
	normalized, err := parseAndValidate(def.Type, value)
	if err != nil {
		return Entry{}, err
	}
	// Encrypt secret values at rest, storing plaintext when no cipher is configured (dev) - the returned Entry always carries the plaintext, so callers like the admin resolver computing "is set" never see ciphertext
	toStore := normalized
	if def.Type == TypeSecret && s.cipher != nil && normalized != "" {
		ct, encErr := s.cipher.Encrypt(normalized)
		if encErr != nil {
			return Entry{}, encErr
		}
		toStore = ct
	}
	repo := s.storeFor(path)
	if repo == nil {
		return Entry{}, ErrStoreUnavailable
	}
	stored, err := repo.Upsert(ctx, scope, path, toStore)
	if err != nil {
		return Entry{}, err
	}
	s.invalidate(ctx)
	s.changed(ctx, def)
	return Entry{
		Definition: def,
		Value:      normalized,
		IsDefault:  false,
		Scope:      stored.Scope,
		UpdatedAt:  stored.UpdatedAt,
	}, nil
}

// Deletes the row at the given scope - the next Get falls back through the resolution chain, typically to the Registry default
func (s *Service) Reset(ctx context.Context, path string, scope Scope) (Entry, error) {
	def, ok := Find(path)
	if !ok {
		return Entry{}, ErrUnknownPath
	}
	if err := validateScope(scope); err != nil {
		return Entry{}, err
	}
	if err := checkScoping(path, scope); err != nil {
		return Entry{}, err
	}
	repo := s.storeFor(path)
	if repo == nil {
		return Entry{}, ErrStoreUnavailable
	}
	if err := repo.Delete(ctx, scope, path); err != nil {
		return Entry{}, err
	}
	s.invalidate(ctx)
	s.changed(ctx, def)
	return s.Get(ctx, path, scope)
}

// One Entry per Registry definition in declaration order at the given scope - orphan DB rows, whose paths were removed from the Registry, are silently skipped, and ListOrphans surfaces them
func (s *Service) ListAll(ctx context.Context, scope Scope) ([]Entry, error) {
	if err := validateScope(scope); err != nil {
		return nil, err
	}
	rows, err := s.rowsInScope(ctx, scope)
	if err != nil {
		return nil, err
	}

	index := make(map[string]StoredValue, len(rows))
	for _, row := range rows {
		index[scopeKey(row.Scope, row.Path)] = row
	}

	rungs := scopeLadder(scope)
	definitions := AllDefinitions()
	entries := make([]Entry, 0, len(definitions))
	for _, def := range definitions {
		matched := false
		for _, rung := range rungs {
			sv, ok := index[scopeKey(rung, def.Path)]
			if !ok {
				continue
			}
			entries = append(entries, Entry{
				Definition: def,
				Value:      sv.Value,
				IsDefault:  false,
				Scope:      rung,
				UpdatedAt:  sv.UpdatedAt,
			})
			matched = true
			break
		}
		if matched {
			continue
		}
		if sv, ok := index[scopeKey(DefaultScope, def.Path)]; ok {
			entries = append(entries, Entry{
				Definition: def,
				Value:      sv.Value,
				IsDefault:  false,
				Scope:      DefaultScope,
				UpdatedAt:  sv.UpdatedAt,
			})
			continue
		}
		entries = append(entries, Entry{
			Definition: def,
			Value:      def.Default,
			IsDefault:  true,
			Scope:      DefaultScope,
		})
	}
	return entries, nil
}

// Only the Registry entries with Public=true, resolved at the caller's scope, for the unauthenticated GraphQL query - every storefront page renders this, so it's read-through-cached per scope (a shared key would serve one channel's values to another) and admin writes drop the cache
func (s *Service) ListPublic(ctx context.Context, scope Scope) ([]Entry, error) {
	if err := validateScope(scope); err != nil {
		return nil, err
	}
	cacheKey := "public:" + scopeKey(scope, "")
	if cached, ok := cache.GetJSON[[]Entry](ctx, s.cache, cacheKey); ok {
		return cached, nil
	}
	all, err := s.ListAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(all))
	for _, e := range all {
		if e.Definition.Public {
			out = append(out, e)
		}
	}
	s.cache.SetJSON(ctx, cacheKey, out, publicConfigCacheTTL)
	return out, nil
}

// The settings page shows both stores at once, which is the whole point of the path carrying its store
func (s *Service) rowsInScope(ctx context.Context, scope Scope) ([]StoredValue, error) {
	rows, err := s.repo.ListInScopes(ctx, scope)
	if err != nil {
		return nil, err
	}
	if s.signals == nil {
		return rows, nil
	}
	sigRows, err := s.signals.ListInScopes(ctx, scope)
	if err != nil {
		// The suite being unreachable must not blank the commerce half - its settings read as their defaults until it is back
		slog.Warn("signals settings unavailable, showing registry defaults for them", "err", err)
		return rows, nil
	}
	return append(rows, sigRows...), nil
}

func (s *Service) ListOrphans(ctx context.Context) ([]StoredValue, error) {
	out := make([]StoredValue, 0)
	for _, repo := range s.stores() {
		rows, err := repo.ListAll(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if _, ok := Find(row.Path); !ok {
				out = append(out, row)
			}
		}
	}
	return out, nil
}

// An orphan has no definition left to say which store it sits in, so pruning walks them
func (s *Service) stores() []Repository {
	if s.signals == nil {
		return []Repository{s.repo}
	}
	return []Repository{s.repo, s.signals}
}

func (s *Service) Prune(ctx context.Context) (int, error) {
	orphans, err := s.ListOrphans(ctx)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(orphans)) // a path in both stores is listed by each, and deleteEverywhere already covers both
	pruned := 0
	for _, o := range orphans {
		key := scopeKey(o.Scope, o.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := s.deleteEverywhere(ctx, o.Scope, o.Path); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}

// deleteEverywhere drops a path from whichever store holds it, which an orphan cannot say for itself
func (s *Service) deleteEverywhere(ctx context.Context, scope Scope, path string) error {
	for _, repo := range s.stores() {
		if err := repo.Delete(ctx, scope, path); err != nil {
			return err
		}
	}
	return nil
}

// --- helpers ---

// Refuses a write the read path would ignore - a channel row on a platform-wide knob, or any
// per-locale row, would sit in the table looking effective
func checkScoping(path string, scope Scope) error {
	if ScopingAllows(path, scope.Kind, scope.Locale != nil) {
		return nil
	}
	if scope.Locale != nil {
		return fmt.Errorf("%w: settings are not per-locale - drop the locale from the scope", ErrInvalidScope)
	}
	return fmt.Errorf("%w: %s is platform-wide - set it on the default scope", ErrInvalidScope, path)
}

func validateScope(scope Scope) error {
	switch scope.Kind {
	case ScopeDefault:
		if scope.ChannelID != nil {
			return fmt.Errorf("%w: default scope cannot include channelID", ErrInvalidScope)
		}
		if scope.Locale != nil {
			return fmt.Errorf("%w: default scope cannot include locale", ErrInvalidScope)
		}
		return nil
	case ScopeChannel:
		if scope.ChannelID == nil {
			return fmt.Errorf("%w: channel scope requires a channelID", ErrInvalidScope)
		}
		if scope.Locale != nil && strings.TrimSpace(*scope.Locale) == "" {
			return fmt.Errorf("%w: locale cannot be empty", ErrInvalidScope)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown scope kind %q", ErrInvalidScope, scope.Kind)
	}
}

// Checks the raw string parses cleanly as the declared Type and returns the normalized serialization
// Int values round-trip through ParseInt/FormatInt so "007" becomes "7", bools canonicalize to true/false, JSON is re-marshaled to strip whitespace, and strings pass through unchanged
func parseAndValidate(t Type, raw string) (string, error) {
	switch t {
	case TypeString, TypeTextarea, TypePhone, TypeSelect, TypeMultiselect, TypeSecret, TypeImage:
		// Basic types pass through - specific validation, a regex for phone or membership for select, can go here or in the UI
		// Secrets are opaque strings encrypted in Set after this normalization, and image values are opaque too, either a URL or a serialized AssetRef
		return raw, nil
	case TypeColor:
		// Empty means unset - otherwise require a #rrggbb hex color so a bad value can never reach the storefront's inline styles, and normalize case
		if strings.TrimSpace(raw) == "" {
			return "", nil
		}
		c := strings.ToLower(strings.TrimSpace(raw))
		if !isHexColor(c) {
			return "", ErrInvalidValue
		}
		return c, nil
	case TypeInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return "", ErrInvalidValue
		}
		return strconv.FormatInt(n, 10), nil
	case TypeBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return "", ErrInvalidValue
		}
		if b {
			return "true", nil
		}
		return "false", nil
	case TypeJSON, TypeWorkingHours, TypeSearchAttributes, TypeSortOptions:
		var tmp any
		if err := json.Unmarshal([]byte(raw), &tmp); err != nil {
			return "", ErrInvalidValue
		}
		out, err := json.Marshal(tmp)
		if err != nil {
			return "", ErrInvalidValue
		}
		return string(out), nil
	}
	return "", errors.New("unknown config type")
}

// A 7-char #rrggbb hex color, case-insensitive - callers lowercase before storing
func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// A cheap composite map key for the in-memory merge in ListAll Most specific first: the exact scope, then the channel-wide row when a
// locale was asked for The default row is the last rung and is handled separately, since it also answers IsDefault
func scopeLadder(scope Scope) []Scope {
	if scope == DefaultScope {
		return nil
	}
	rungs := []Scope{scope}
	if scope.Kind == ScopeChannel && scope.Locale != nil {
		rungs = append(rungs, Scope{Kind: ScopeChannel, ChannelID: scope.ChannelID})
	}
	return rungs
}

// Splits candidate rows into the best match for this scope and the platform default
func pickRow(candidates []StoredValue, scope Scope) (scoped, dflt *StoredValue) {
	byScope := make(map[string]StoredValue, len(candidates))
	for _, sv := range candidates {
		byScope[scopeKey(sv.Scope, "")] = sv
	}
	if sv, ok := byScope[scopeKey(DefaultScope, "")]; ok {
		d := sv
		dflt = &d
	}
	for _, rung := range scopeLadder(scope) {
		if sv, ok := byScope[scopeKey(rung, "")]; ok {
			r := sv
			return &r, dflt
		}
	}
	return nil, dflt
}

func scopeKey(scope Scope, path string) string {
	channelID := ""
	if scope.ChannelID != nil {
		channelID = scope.ChannelID.String()
	}
	locale := ""
	if scope.Locale != nil {
		locale = *scope.Locale
	}
	return scope.Kind + ":" + channelID + ":" + locale + ":" + path
}
