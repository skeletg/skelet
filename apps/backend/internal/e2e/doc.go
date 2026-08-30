// Package e2e drives the money path - place, capture, refund, cancel - through the real
// composition-root graph against a live Postgres, then asserts the invariants the domain
// promises: balanced ledgers, per-target refund ceilings, reservation counters back at zero,
// tenders returned exactly once
//
// The scenarios are env-gated like the other DB suites: without DATABASE_URL and SIGNALS_DB_URL
// they skip, so a bare `go test ./...` stays green while CI (which migrates both databases and
// exports both DSNs) runs them on every push. Locally: bin/go test ./internal/e2e/...
//
// A DSN alone is NOT consent - the suite writes orders and drains every due outbox row, so it
// refuses any database not explicitly marked disposable:
// ALTER DATABASE <name> SET skelet.e2e = 'allowed' (CI marks its own; production never has it)
//
// Determinism comes from two production seams instead of sleeps: outbox.DrainPending dispatches
// due rows inline and returns a count (loop until 0 - handlers may enqueue), and events.Bus.
// WaitIdle waits out detached handlers without ending the bus the way Drain would
package e2e
