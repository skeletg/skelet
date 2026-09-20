// Package pluginmigrate runs Tier 1 plugin SQL migrations - separate from golang-migrate, which owns core's internal/db/migrations A separate runner exists because golang-migrate can't model multiple sources, plugin sequence numbers mustn't collide with core's, and the SQL has to be embedded in the plugin binary so a plugin stays self-contained
// Progress is tracked per-namespace in plugin_migrations Core migrations always run first - Within a plugin, files apply in lexicographic order; across plugins it's registration order, i.e. import order in main.go - stable per build, not portable across builds, and plugins may NOT reference each other's tables
package pluginmigrate

import (
	"database/sql"
	"errors"
	"io/fs"
	"sync"
)

type Registration struct {
	Namespace string
	FS        fs.FS
	Root      string // directory inside FS containing the .sql files
}

var (
	regMu  sync.RWMutex
	regs   []Registration
	regIdx = map[string]int{} // namespace → index in regs (for dup detection)
)

// Panics on a duplicate namespace - called by internal/plugin/host.go in response to sdk.RegisterMigrations
func Register(reg Registration) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := regIdx[reg.Namespace]; exists {
		panic("pluginmigrate: namespace " + reg.Namespace + " already registered")
	}
	regIdx[reg.Namespace] = len(regs)
	regs = append(regs, reg)
}

func AllRegistrations() []Registration {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Registration, len(regs))
	copy(out, regs)
	return out
}

func FindRegistration(namespace string) (Registration, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	idx, ok := regIdx[namespace]
	if !ok {
		return Registration{}, false
	}
	return regs[idx], true
}

// AppliedVersions is every (NNNN) row in plugin_migrations for the namespace and PendingVersions is the FS files not yet applied - either may be empty
type Status struct {
	Namespace        string
	AppliedVersions  []int
	PendingVersions  []int
	HighestApplied   int  // 0 when nothing applied
	HighestAvailable int  // 0 when no files in FS
	HasMigrations    bool // FS contains at least one parseable file
}

// Distinct from "registered but has no migrations yet", which shows up as a Status with HasMigrations=false
var ErrNamespaceNotRegistered = errors.New("pluginmigrate: namespace not registered")

type Migration struct {
	Version  int
	Name     string
	UpSQL    string
	DownSQL  string // empty when no .down.sql sibling exists
	UpPath   string // relative path inside FS, for error messages
	DownPath string
}

// Separate from the registry so tests can swap in fake registrations
type Runner struct {
	db *sql.DB
}

func New(db *sql.DB) *Runner { return &Runner{db: db} }

func ResetForTest() {
	regMu.Lock()
	defer regMu.Unlock()
	regs = nil
	regIdx = map[string]int{}
}
