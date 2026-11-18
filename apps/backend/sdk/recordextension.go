package sdk

import "slices"

// The records whose saves carry a part for plugins - what RecordSaving.SavingRecord answers
const (
	RecordProduct  = "product"
	RecordCustomer = "customer"
)

// A plugin's part of a product's or a customer's save: what the save's `extensions` holds under the plugin's slug.
// Declaring it is what lets the part in - core refuses a save carrying a part for a plugin that declared none for
// that record, before anything is written, so a mistyped slug or a plugin switched off never looks like a save that
// took
type RecordExtension struct {
	Record string // RecordProduct or RecordCustomer
	// What an admin needs to send the part, on top of the record's own permission - one of the plugin's own
	// permissions, usually. Empty asks for nothing more: anyone who may save the record may set your part
	Permission string
}

type RegisteredRecordExtension struct {
	PluginSlug string
	Extension  RecordExtension
}

var recordExtensions registry[RegisteredRecordExtension]

// Panics on a record that carries no parts, or one the plugin already declared a part of
func (pl *Plugin) RegisterRecordExtension(e RecordExtension) {
	if !slices.Contains([]string{RecordProduct, RecordCustomer}, e.Record) {
		panic("sdk.RegisterRecordExtension: no record " + e.Record + " takes a part of its save")
	}
	recordExtensions.add(RegisteredRecordExtension{PluginSlug: pl.slug, Extension: e},
		func(r RegisteredRecordExtension) bool {
			return r.PluginSlug == pl.slug && r.Extension.Record == e.Record
		},
		"sdk.RegisterRecordExtension: "+pl.slug+" already declared its part of a "+e.Record+"'s save")
}

// RecordExtensions is every part of a save the compiled-in plugins declared, in the order they did
func RecordExtensions() []RegisteredRecordExtension { return recordExtensions.all() }

// ValidSlug reports whether s has a plugin slug's shape - what keys a save's extensions
func ValidSlug(s string) bool { return slugPattern.MatchString(s) }
