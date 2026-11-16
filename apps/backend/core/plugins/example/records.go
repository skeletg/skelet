package example

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

var recordsResource = sdk.DataResource{
	Slug: "records", Label: "Example records", Description: "The example plugin's notes.", Global: true, // one table for the whole installation, so a run needs an unscoped grant
	ViewPermission: "example.manage", ImportPermission: "example.manage",
	Fields: []sdk.DataField{
		{Name: "id", Type: sdk.DataInteger, Identifier: true, Description: "Blank adds a record"},
		{Name: "note", Required: true, Example: "Reconciled by hand"},
		{Name: "created_at", Type: sdk.DataDate, ReadOnly: true},
	},
	Export: func(ctx context.Context, _ sdk.DataRun, emit func(sdk.DataRecord) error) error {
		db, err := sdk.Database()
		if err != nil {
			return err
		}
		rows, err := db.QueryContext(ctx, `SELECT id, note, created_at FROM example_plugin_records ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				id   int64
				note string
				at   time.Time
			)
			if err := rows.Scan(&id, &note, &at); err != nil {
				return err
			}
			if err := emit(sdk.DataRecord{"id": id, "note": note, "created_at": at}); err != nil {
				return err
			}
		}
		return rows.Err()
	},
	Import: func(ctx context.Context, run sdk.DataRun, row sdk.DataRow) (sdk.DataOutcome, error) {
		if !row.Has("note") {
			row.Fail("note is required")
			return sdk.DataSkipped, nil
		}
		outcome := sdk.DataCreated
		if row.Has("id") {
			outcome = sdk.DataUpdated
		}
		if run.DryRun {
			return outcome, nil
		}
		db, err := sdk.Database()
		if err != nil {
			return sdk.DataSkipped, err
		}
		if outcome == sdk.DataCreated {
			_, err = db.ExecContext(ctx, `INSERT INTO example_plugin_records (note) VALUES ($1)`, row.Str("note"))
			return outcome, err
		}
		res, err := db.ExecContext(ctx, `UPDATE example_plugin_records SET note = $2 WHERE id = $1`, row.Int("id"), row.Str("note"))
		if err != nil {
			return sdk.DataSkipped, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return sdk.DataSkipped, sdk.Invalid("no record %d", row.Int("id"))
		}
		return outcome, nil
	},
}
