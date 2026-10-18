// Package e2e drives the money path - place, capture, refund, cancel - through the real composition-root graph, asserting balanced ledgers, refund ceilings and tenders returned once A DSN alone is not consent: it refuses any database without ALTER DATABASE <name> SET skelet.e2e = 'allowed' Determinism comes from outbox.DrainPending and events.Bus.WaitIdle rather than sleeps
package e2e
