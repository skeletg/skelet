package example

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Notes kept against a customer leave with their data export and go with an erasure
var customerNotes = sdk.CustomerData{
	Key: "notes",
	Export: func(ctx context.Context, who sdk.CustomerDataSubject) (any, error) {
		db, err := sdk.Database()
		if err != nil {
			return nil, err
		}
		rows, err := db.QueryContext(ctx, `SELECT note FROM example_plugin_customer_notes WHERE customer_id = $1 ORDER BY id`, who.CustomerID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var notes []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return nil, err
			}
			notes = append(notes, n)
		}
		if len(notes) == 0 {
			return nil, rows.Err()
		}
		return notes, rows.Err()
	},
	Erase: func(ctx context.Context, who sdk.CustomerDataSubject) error {
		db, err := sdk.Database()
		if err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `DELETE FROM example_plugin_customer_notes WHERE customer_id = $1`, who.CustomerID)
		return err
	},
}

// The customers list offers "Has notes" in its toolbar, narrowed to the customers this table holds a note for
var customersWithNotes = sdk.ListFilter{
	List:    sdk.ListCustomers,
	Key:     "notes",
	Label:   "Notes",
	Options: []sdk.ListFilterOption{{Value: "any", Label: "Has notes"}},
	Match: func(ctx context.Context, _ []string) ([]uuid.UUID, error) {
		db, err := sdk.Database()
		if err != nil {
			return nil, err
		}
		rows, err := db.QueryContext(ctx, `SELECT DISTINCT customer_id FROM example_plugin_customer_notes`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	},
}
