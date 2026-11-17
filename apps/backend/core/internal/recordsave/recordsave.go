// Package recordsave is the sync event a core record raises as it's written, inside the transaction that writes it -
// carrying what the save's input held for plugins, so a plugin's own rows commit or roll back with the record's
package recordsave

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	EventProductSaving  = "product.saving"
	EventCustomerSaving = "customer.saving"
)

// What Saving.Record holds
const (
	RecordProduct  = "product"
	RecordCustomer = "customer"
)

// The most a save's extensions may hold all told, as JSON
const MaxExtensions = 64 << 10

// Saving is a record being created or updated, after its row is written and before the transaction commits. It's for
// the plugins compiled in, which write their rows through Tx - not a webhook event: an endpoint called from in here
// would hold the record's lock for its round trip, read every plugin's part, and never learn whether the save it
// answered for went on to commit. The updating events are a webhook's say, before the transaction opens
type Saving struct {
	Event      string                     `json:"-"`
	Record     string                     `json:"record"` // product, customer
	RecordID   uuid.UUID                  `json:"recordId"`
	ChannelID  uuid.UUID                  `json:"channelId,omitzero"` // the record's channel, for one that has one
	Created    bool                       `json:"created"`
	Extensions map[string]json.RawMessage `json:"extensions"`
	Tx         *sql.Tx                    `json:"-"` // nil only in a unit test without a database
}

func (e *Saving) Type() string              { return e.Event }
func (e *Saving) EventChannelID() uuid.UUID { return e.ChannelID }
func (e *Saving) SavingRecord() string      { return e.Record }
func (e *Saving) SavingRecordID() uuid.UUID { return e.RecordID }
func (e *Saving) SavingCreated() bool       { return e.Created }
func (e *Saving) SavingTx() *sql.Tx         { return e.Tx }
func (e *Saving) SavingExtension(key string) (json.RawMessage, bool) {
	v, ok := e.Extensions[key]
	return v, ok
}

// Check refuses extensions a save shouldn't carry: a key that isn't a plugin's slug, or more than MaxExtensions of
// JSON. Whether a plugin takes the part, and whether the admin may send it, is the API's to ask - see
// plugin.RecordExtensionPermissions
func Check(ext map[string]json.RawMessage) error {
	size := 0
	for k, v := range ext {
		if !sdk.ValidSlug(k) {
			return apperr.New("EXTENSION_KEY_INVALID", apperr.ClassValidation, fmt.Sprintf("extension %q needs a plugin's slug as its key", k))
		}
		size += len(k) + len(v)
	}
	if size > MaxExtensions {
		return apperr.New("EXTENSIONS_TOO_LARGE", apperr.ClassValidation, fmt.Sprintf("extensions are %d bytes, past the %d a save carries", size, MaxExtensions))
	}
	return nil
}
