package tax

// Accumulates tax as effects apply, one entry per contribution - RateBPS is the percentage an effect used, 0 for fixed effects, surfaced only for display and audit - Compound effects read Total() as their running base
type Ledger struct {
	entries []LedgerEntry
}

type LedgerEntry struct {
	Cents   int64
	RateBPS int32
}

func (l *Ledger) Add(cents int64, rateBPS int32) {
	if cents == 0 {
		return
	}
	l.entries = append(l.entries, LedgerEntry{Cents: cents, RateBPS: rateBPS})
}

// The running tax sum - the base for compound effects
func (l *Ledger) Total() int64 {
	var t int64
	for _, e := range l.entries {
		t += e.Cents
	}
	return t
}

func (l *Ledger) mark() int { return len(l.entries) }

func (l *Ledger) since(mark int) []LedgerEntry {
	if mark < 0 || mark > len(l.entries) {
		return nil
	}
	return l.entries[mark:]
}
