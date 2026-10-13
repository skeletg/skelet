// Package permission is a code-defined registry for admin and integration permissions, mirroring the config package's Register / Validate pattern Domain packages declare theirs in their own init(), then main.go calls Validate() for structural integrity and Sync() to make permission_groups and permissions reflect the registry - Sync also grants newly-registered permissions to the "administrator" role so it always holds everything Sync is idempotent and safe on every startup - It never deletes, so permissions removed from the registry sit in the DB as inert orphans that no resolver references
package permission

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A top-level grouping in the admin UI's permissions panel, e.g. "Products" or "Configuration"
type Group struct {
	Slug      string
	Name      string
	SortOrder int32
}

type Definition struct {
	Group string // references a registered Group.Slug
	Slug  string // the auth guard key (e.g. "products.view")
	Name  string // human-readable label
	// What holding this permission lets someone do, where the name alone doesn't say - "View Products" restates the slug, "can issue refunds
	// against any order" is the useful part Optional, so it can be backfilled where it earns its place rather than everywhere
	Description string
	SortOrder   int32
}

var (
	mu                sync.RWMutex
	registeredGroups  []Group
	registeredPermiss []Definition
	groupIdx          = map[string]int{}
	permIdx           = map[string]int{}
)

// Panics on a duplicate slug - two packages declaring the same one is a bug that should surface immediately
func RegisterGroup(g Group) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := groupIdx[g.Slug]; exists {
		panic(fmt.Sprintf("permission: group %q already registered", g.Slug))
	}
	registerGroupLocked(g)
}

// For a group several registrants may share (plugins adding to one group) - the check and the insert hold one lock
func RegisterGroupIfAbsent(g Group) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := groupIdx[g.Slug]; !exists {
		registerGroupLocked(g)
	}
}

func registerGroupLocked(g Group) {
	groupIdx[g.Slug] = len(registeredGroups)
	registeredGroups = append(registeredGroups, g)
}

// Panics on a duplicate slug, and does NOT check that the referenced group exists - call Validate() once every package has registered
func Register(d Definition) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := permIdx[d.Slug]; exists {
		panic(fmt.Sprintf("permission: slug %q already registered", d.Slug))
	}
	permIdx[d.Slug] = len(registeredPermiss)
	registeredPermiss = append(registeredPermiss, d)
}

// Checks every registered permission references a registered group - call once at startup, after every init() has run
func Validate() error {
	mu.RLock()
	defer mu.RUnlock()
	var problems []string
	for _, p := range registeredPermiss {
		if _, ok := findGroupLocked(p.Group); !ok {
			problems = append(problems, fmt.Sprintf("permission %q references unknown group %q", p.Slug, p.Group))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("permission registry invalid: %s", strings.Join(problems, "; "))
}

func AllGroups() []Group {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Group, len(registeredGroups))
	copy(out, registeredGroups)
	return out
}

// A copy of the registered permissions in registration order, which is how the admin UI renders them - grouped by Group slug, then by SortOrder within each
func AllPermissions() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Definition, len(registeredPermiss))
	copy(out, registeredPermiss)
	return out
}

func Find(slug string) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if i, ok := permIdx[slug]; ok {
		return registeredPermiss[i], true
	}
	return Definition{}, false
}

func FindGroup(slug string) (Group, bool) {
	mu.RLock()
	defer mu.RUnlock()
	return findGroupLocked(slug)
}

// Makes the DB reflect the registered groups and permissions in a single transaction - upsert every group, upsert every permission against the group's DB id, then grant everything to the administrator role - Idempotent, and never deletes
func Sync(ctx context.Context, queries *db.Queries, txMgr *txrunner.Manager) error {
	mu.RLock()
	groups := make([]Group, len(registeredGroups))
	copy(groups, registeredGroups)
	perms := make([]Definition, len(registeredPermiss))
	copy(perms, registeredPermiss)
	mu.RUnlock()

	return txMgr.InTransaction(ctx, func(t *sql.Tx) error {
		q := queries.WithTx(t)

		// Upsert groups first so permissions can resolve group_id
		groupIDBySlug := make(map[string]uuid.UUID, len(groups))
		for _, g := range groups {
			row, err := q.UpsertPermissionGroup(ctx, db.UpsertPermissionGroupParams{
				Slug:      g.Slug,
				Name:      g.Name,
				SortOrder: g.SortOrder,
			})
			if err != nil {
				return fmt.Errorf("upsert permission group %q: %w", g.Slug, err)
			}
			groupIDBySlug[g.Slug] = row.ID
		}

		for _, p := range perms {
			groupID, ok := groupIDBySlug[p.Group]
			if !ok {
				return fmt.Errorf("permission %q references unknown group %q (Validate should have caught this)", p.Slug, p.Group)
			}
			if _, err := q.UpsertPermission(ctx, db.UpsertPermissionParams{
				GroupID:   groupID,
				Slug:      p.Slug,
				Name:      p.Name,
				SortOrder: p.SortOrder,
			}); err != nil {
				return fmt.Errorf("upsert permission %q: %w", p.Slug, err)
			}
		}

		// Grant everything to the administrator role, so newly-registered permissions reach administrators without manual SQL
		if err := q.GrantAllPermissionsToAdministrator(ctx); err != nil {
			return fmt.Errorf("grant permissions to administrator role: %w", err)
		}
		return nil
	})
}

func findGroupLocked(slug string) (Group, bool) {
	if i, ok := groupIdx[slug]; ok {
		return registeredGroups[i], true
	}
	return Group{}, false
}
