package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/recordsave"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

var (
	savingOnce   sync.Once
	savingPlugin = sdk.NewPluginForTest("e2e-saving")
	savingFor    sync.Map // the records this scenario saves - every other scenario's saves go untouched
)

// Stands in for a plugin that keeps a note per record in its own table, written in the save's own transaction
func subscribeSavingPlugin(h *harness, t *testing.T) {
	t.Helper()
	if _, err := h.D.DB.ExecContext(h.Ctx, `CREATE TABLE IF NOT EXISTS e2e_saving_notes (record_id uuid PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	savingOnce.Do(func() {
		for _, ev := range []string{recordsave.EventProductSaving, recordsave.EventCustomerSaving} {
			h.D.Bus.SubscribeSync(ev, 100, func(ctx context.Context, e events.Event) error {
				save, ok := e.(sdk.RecordSaving)
				if !ok {
					return nil
				}
				if _, mine := savingFor.Load(save.SavingRecordID()); !mine && !save.SavingCreated() {
					return nil
				}
				var part struct {
					Note string `json:"note"`
				}
				if given, err := savingPlugin.Extension(save, &part); !given || err != nil {
					return err
				}
				if part.Note == "refuse" {
					return sdk.Invalid("the note refuses the save")
				}
				_, err := save.SavingTx().ExecContext(ctx,
					`INSERT INTO e2e_saving_notes (record_id, note) VALUES ($1, $2) ON CONFLICT (record_id) DO UPDATE SET note = $2`,
					save.SavingRecordID(), part.Note)
				return err
			})
		}
	})
}

func (h *harness) savedNote(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var note string
	err := h.D.DB.QueryRowContext(h.Ctx, `SELECT note FROM e2e_saving_notes WHERE record_id = $1`, id).Scan(&note)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	return note
}

func ext(note string) map[string]json.RawMessage {
	raw, _ := json.Marshal(map[string]string{"note": note})
	return map[string]json.RawMessage{"e2e-saving": raw}
}

// Scenario: a plugin's part of a product save lands in its own table in the save's transaction, and its refusal rolls
// the product's own change back with it
func TestAPluginsPartOfAProductSaveCommitsAndRollsBackWithIt(t *testing.T) {
	h := newHarness(t)
	subscribeSavingPlugin(h, t)
	f := h.NewStore(t, 10, 2000)
	id := f.Product.ID
	savingFor.Store(id, true)

	sku := "SAVED-" + uuid.NewString()[:8]
	if _, err := h.D.Catalog.UpdateProduct(h.Ctx, catalog.UpdateProductInput{ID: id, SKU: &sku, Extensions: ext("first")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := h.savedNote(t, id); got != "first" {
		t.Errorf("note %q after the save, want first", got)
	}

	refused := "REFUSED-" + uuid.NewString()[:8]
	_, err := h.D.Catalog.UpdateProduct(h.Ctx, catalog.UpdateProductInput{ID: id, SKU: &refused, Extensions: ext("refuse")})
	if err == nil {
		t.Fatal("a refused save went through")
	}
	p, err := h.D.Catalog.GetProduct(h.Ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.SKU != sku || h.savedNote(t, id) != "first" {
		t.Errorf("after the refusal: sku %q note %q - want both as the last good save left them", p.SKU, h.savedNote(t, id))
	}

	bad := map[string]json.RawMessage{"Not A Slug": json.RawMessage(`{}`)}
	if _, err := h.D.Catalog.UpdateProduct(h.Ctx, catalog.UpdateProductInput{ID: id, Extensions: bad}); err == nil {
		t.Error("a malformed extension key was taken")
	}
}

// Scenario: a customer created with a plugin's part carries it into the plugin's table, marked as a creation
func TestAPluginsPartOfACustomerSaveCommitsWithIt(t *testing.T) {
	h := newHarness(t)
	subscribeSavingPlugin(h, t)
	f := h.NewStore(t, 10, 2000)
	c, err := h.D.Customer.AdminCreateCustomer(h.Ctx, customer.AdminCreateCustomerInput{
		ChannelID: f.Channel.ID, Email: "saving-" + uuid.NewString()[:8] + "@e2e.test", Password: "correct horse battery",
		FirstName: "Ana", LastName: "Saving", Extensions: ext("vip"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := h.savedNote(t, c.ID); got != "vip" {
		t.Errorf("note %q after the create, want vip", got)
	}
}
