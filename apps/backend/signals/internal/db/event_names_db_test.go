package db

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/migfile"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The GA4 rename rewrites every saved definition that names an old event - wherever in the JSON it sits - and nothing
// else: a title or a filter value that reads like an event name stays. The up section is safe to run again, which is
// how it is driven here, over rows written in the old vocabulary
func TestTheRenameRewritesEverySavedDefinition(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ch := uuid.New()
	var board, segment uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(conn.QueryRow(`INSERT INTO boards (channel_id, name) VALUES ($1, 'old names') RETURNING id`, ch).Scan(&board))
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM boards WHERE id = $1`, board) })
	_, err := conn.Exec(`INSERT INTO board_reports (board_id, position, title, definition) VALUES
		($1, 0, 'lead', '{"kind":"funnel","steps":["page_view","cart_add","order_placed"],"filters":[{"step":0,"key":"device","value":"registered"}]}'),
		($1, 1, 'Repeat', '{"kind":"retention","startEvent":"order_placed","returnEvent":"registered"}')`, board)
	must(err)
	_, err = conn.Exec(`INSERT INTO board_drafts (board_id, admin_id, payload, base_updated_at) VALUES ($1, $2, '{"Name":"x","Reports":[{"Title":"t","Definition":{"kind":"insights","event":"wishlist_add"}}]}', now())`, board, uuid.New())
	must(err)
	must(conn.QueryRow(`INSERT INTO customer_segments (slug, name, definition) VALUES ($1, 'old', '{"channelId":"","root":{"kind":"group","op":"and","children":[{"kind":"event","event":"$search"},{"kind":"event","event":"view_item"}]}}') RETURNING id`, "old-"+uuid.NewString()[:8]).Scan(&segment))
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_segments WHERE id = $1`, segment) })
	key := "exp-" + uuid.NewString()[:8]
	_, err = conn.Exec(`INSERT INTO experiments (channel_id, key, name, goal_event) VALUES ($1, $2, 'e', 'lead')`, ch, key)
	must(err)
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM experiments WHERE key = $1`, key) })

	f, err := migfile.Read("migrations/20261115221602_ga4_event_names.sql")
	must(err)
	tx, err := conn.Begin()
	must(err)
	if _, err := tx.Exec(f.Up()); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	must(tx.Commit())

	var reports []string
	rows, err := conn.Query(`SELECT title || ' ' || definition::text FROM board_reports WHERE board_id = $1 ORDER BY position`, board)
	must(err)
	for rows.Next() {
		var s string
		must(rows.Scan(&s))
		reports = append(reports, s)
	}
	must(rows.Err())
	var funnel struct {
		Steps   []string
		Filters []struct{ Value string }
	}
	must(json.Unmarshal([]byte(strings.TrimPrefix(reports[0], "lead ")), &funnel))
	if strings.Join(funnel.Steps, ",") != "page_view,add_to_cart,purchase" || funnel.Filters[0].Value != "registered" || !strings.HasPrefix(reports[0], "lead ") {
		t.Errorf("funnel became %s - want its steps renamed and its title and filter value kept", reports[0])
	}
	if !strings.Contains(reports[1], `"startEvent": "purchase"`) || !strings.Contains(reports[1], `"returnEvent": "sign_up"`) {
		t.Errorf("retention became %s", reports[1])
	}
	var draft, def, goal string
	must(conn.QueryRow(`SELECT payload::text FROM board_drafts WHERE board_id = $1`, board).Scan(&draft))
	must(conn.QueryRow(`SELECT definition FROM customer_segments WHERE id = $1`, segment).Scan(&def))
	must(conn.QueryRow(`SELECT goal_event FROM experiments WHERE key = $1`, key).Scan(&goal))
	if !strings.Contains(draft, `"event": "add_to_wishlist"`) {
		t.Errorf("draft became %s", draft)
	}
	if !strings.Contains(def, `"event": "search"`) || !strings.Contains(def, `"event": "view_item"`) {
		t.Errorf("segment became %s", def)
	}
	if goal != "generate_lead" {
		t.Errorf("experiment goal became %q", goal)
	}
}
