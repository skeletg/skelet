package sdk

import (
	"context"
	"database/sql"
	"errors"
)

// The application database, for a plugin that owns tables - RegisterMigrations creates them at boot,
// this reads and writes them on the request path
//
// The handle is the SAME pool core uses, not a sandbox: a plugin can read and write every core table,
// and nothing here stops it. That is the Tier 1 bargain - a plugin is code you chose to compile in with
// full privileges, and db.read / db.write in the manifest are a disclosure, not a fence. Declare them
//
// Scope your SQL to tables your own migrations created. Reading a core table is a coupling to a schema
// that is free to change between releases; writing one goes behind the owning service's invariants,
// its events and its cache invalidation, and the bug surfaces somewhere else entirely. Where core has
// an event or a registration seam, use that instead
//
// Prefix every table with the plugin slug for the same reason SDL fields are prefixed - two plugins
// creating `invoices` is a collision that migrations discover at boot on someone else's install
//
// To write inside a core transaction rather than your own, don't use this: subscribe to the sync event
// that carries the *sql.Tx (order.placing, order.tendering) and join it, so your rows commit or roll
// back with the operation that caused them
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Returned before the host has wired the pool, which is a boot-order bug rather than a runtime state -
// registration runs in init(), long before the database exists, so resolve the handle on first USE
// (a sync.Once, or a call per request), never in init()
var ErrDBUnavailable = errors.New("sdk: database unavailable")

// The pool, once the host has one
//
//	func (s *store) db() (sdk.DB, error) { return sdk.Database() }
func Database() (DB, error) {
	h := hostNow()
	if h == nil {
		return nil, ErrDBUnavailable
	}
	return h.Database()
}
