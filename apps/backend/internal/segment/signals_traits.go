package segment

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// customer_traits lives in the signals database, so a trait condition is resolved to an id set here and injected as a bind arg rather than being a subquery in the customer scan
// Same safety property as the main compiler, since every field and operator comes from the allowlists and never from operator input

// SetSignalsDB wires the read that resolves trait conditions - without it those conditions match nobody, which is what a segment on stale analytics should do
func (s *Service) SetSignalsDB(dbc *sql.DB) { s.sigDB = dbc }

// resolveSignalsConditions runs each trait condition against the signals database, returning the ids by condition index
func (s *Service) resolveSignalsConditions(ctx context.Context, rules *Rules) (map[int][]uuid.UUID, error) {
	conds := rules.SignalsConditions()
	if len(conds) == 0 {
		return nil, nil
	}
	if s.sigDB == nil {
		return nil, fmt.Errorf("segment has a trait condition but no signals database is wired")
	}
	out := make(map[int][]uuid.UUID, len(conds))
	for i, c := range conds {
		var (
			query string
			arg   any
		)
		switch c.Kind {
		case "trait":
			query = fmt.Sprintf("SELECT customer_id FROM customer_traits WHERE %s %s $1", traitFields[c.Field], numericOps[c.Op])
			arg = c.Value
		case "lifecycle":
			op := "="
			if c.Op == "neq" {
				op = "<>"
			}
			query = fmt.Sprintf("SELECT customer_id FROM customer_traits WHERE lifecycle_stage %s $1", op)
			arg = c.Stage
		}
		ids, err := s.scanIDs(ctx, query, arg)
		if err != nil {
			return nil, fmt.Errorf("resolve %s condition: %w", c.Kind, err)
		}
		out[i] = ids
	}
	return out, nil
}

func (s *Service) scanIDs(ctx context.Context, query string, arg any) ([]uuid.UUID, error) {
	rows, err := s.sigDB.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
