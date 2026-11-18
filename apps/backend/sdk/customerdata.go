package sdk

import (
	"context"

	"github.com/google/uuid"
)

// What a plugin keeps about a customer, so it leaves with the customer's own data - a data export carries it, and an
// erasure scrubs it. A table with a foreign key to customers and ON DELETE CASCADE goes when the customer is deleted;
// an erasure keeps the customer's row and blanks it, so that's the one Erase is for
type CustomerData struct {
	Key string // lowercase, stable - the export's section is pl.Key of it ("<plugin>:warranties")
	// What the export carries, marshalled to JSON - nil for nothing. An error fails the whole export, since one missing
	// a section answers the request wrongly while looking complete
	Export func(ctx context.Context, who CustomerDataSubject) (any, error)
	// Removes or blanks what the plugin keeps. It runs after the customer is erased or deleted and again, on a widening
	// schedule, until it returns nil - core keeps the erasure owed for as long as that takes - so it must be safe to
	// repeat; it runs whether or not the plugin is on, since the data outlives that. A subject's Email is empty when
	// the account's address wasn't confirmed as theirs: never match your rows on an empty one
	Erase func(ctx context.Context, who CustomerDataSubject) error
}

// The customer, as they were - an erasure hands the address the account had before it was blanked, for rows a guest
// left under it
type CustomerDataSubject struct {
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Email      string
}

type RegisteredCustomerData struct {
	PluginSlug string
	Data       CustomerData
}

// Key is the section's key as the export carries it, "<plugin>:<key>"
func (r RegisteredCustomerData) Key() string { return r.PluginSlug + ":" + r.Data.Key }

var customerData registry[RegisteredCustomerData]

// Panics on a malformed registration or a key the plugin already uses
func (pl *Plugin) RegisterCustomerData(d CustomerData) {
	switch {
	case !hookKey.MatchString(d.Key):
		panic("sdk.RegisterCustomerData: key " + d.Key + hookKeyRule)
	case d.Export == nil && d.Erase == nil:
		panic("sdk.RegisterCustomerData: " + d.Key + " needs an Export, an Erase or both")
	}
	customerData.add(RegisteredCustomerData{PluginSlug: pl.slug, Data: d},
		func(r RegisteredCustomerData) bool { return r.PluginSlug == pl.slug && r.Data.Key == d.Key },
		"sdk.RegisterCustomerData: "+pl.slug+" already keeps "+d.Key)
}

// CustomerDataSets is every registration the compiled-in plugins made, in the order they did
func CustomerDataSets() []RegisteredCustomerData { return customerData.all() }
