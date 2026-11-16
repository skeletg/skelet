package sdk

import (
	"context"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
)

// How a column is parsed and rendered - money is whole minor units, a list is pipe-separated
type DataFieldType string

const (
	DataText    DataFieldType = "text"
	DataInteger DataFieldType = "integer"
	DataNumber  DataFieldType = "number"
	DataBool    DataFieldType = "bool"
	DataDate    DataFieldType = "date"
	DataMoney   DataFieldType = "money"
	DataList    DataFieldType = "list"
	DataJSON    DataFieldType = "json"
)

// DataFieldTypes is every column type the data hub reads
func DataFieldTypes() []DataFieldType {
	return []DataFieldType{DataText, DataInteger, DataNumber, DataBool, DataDate, DataMoney, DataList, DataJSON}
}

type DataField struct {
	Name        string // the column header, lowercase
	Label       string
	Description string
	Example     string
	Type        DataFieldType // empty is text
	Required    bool          // to create a record - an update may leave it out
	Identifier  bool          // part of the key rows upsert on - an importable resource needs one
	ReadOnly    bool          // exported for context, ignored on import
	Enum        []string
}

type DataChoice struct {
	Value, Label string
}

type DataOptionType string

const (
	DataOptionText   DataOptionType = "string"
	DataOptionInt    DataOptionType = "int"
	DataOptionBool   DataOptionType = "bool"
	DataOptionSelect DataOptionType = "select"
	DataOptionDate   DataOptionType = "date"
)

type DataOption struct {
	Name        string
	Label       string
	Description string
	Type        DataOptionType
	Required    bool
	Default     string
	Choices     []DataChoice
}

// What a run was asked for - a dry run checks each row and writes nothing
type DataRun struct {
	ChannelID uuid.UUID // uuid.Nil for the whole installation
	Locale    string
	DryRun    bool
	Options   map[string]string
}

// One decoded row - a malformed cell fails the row by itself, and Fail fails it with your own reason. A Required field
// is yours to insist on when the row creates a record (Require), since only you know whether it does
type DataRow interface {
	Has(name string) bool
	Str(name string) string
	Int(name string) int64
	Float(name string) float64
	Bool(name string) bool
	Time(name string) *time.Time
	List(name string) []string
	JSON(name string, into any) bool // false on an empty cell, and on one that isn't valid JSON, which fails the row
	Require(names ...string) bool    // fails the row unless every named column carries a value
	Fail(format string, args ...any)
}

type DataOutcome int

const (
	DataSkipped DataOutcome = iota
	DataCreated
	DataUpdated
)

// A json field's value is marshalled unless it's a string already
type DataRecord map[string]any

// Core owns the file: parsing, validation against the fields, dry runs, error isolation and progress. Import runs
// once per row, and an error fails only that row; Export streams records until it returns
type DataResource struct {
	Slug  string // lowercase, unique within the plugin - core files it as "<plugin>.<slug>"
	Label string
	// Global is for rows the whole installation shares, which a run needs an unscoped grant for. A resource left
	// channel-scoped has to keep to run.ChannelID itself: core checks the grant on that channel, not what you read
	Global           bool
	Description      string
	Fields           []DataField
	Options          []DataOption
	ViewPermission   string // a permission the plugin registered, gating export
	ImportPermission string // empty leaves the resource export-only
	Export           func(ctx context.Context, run DataRun, emit func(DataRecord) error) error
	Import           func(ctx context.Context, run DataRun, row DataRow) (DataOutcome, error)
}

type RegisteredDataResource struct {
	PluginSlug string
	Resource   DataResource
}

var (
	dataResources registry[RegisteredDataResource]
	dataColumn    = regexp.MustCompile(`^[a-z0-9_:.-]+$`)
)

// Lists the resource in the dashboard's data hub - panics on a malformed one or a slug the plugin already used
func (pl *Plugin) RegisterDataResource(r DataResource) {
	identified := slices.ContainsFunc(r.Fields, func(f DataField) bool { return f.Identifier })
	switch {
	case !hookKey.MatchString(r.Slug):
		panic("sdk.RegisterDataResource: slug " + r.Slug + hookKeyRule)
	case r.Label == "" || r.ViewPermission == "" || len(r.Fields) == 0:
		panic("sdk.RegisterDataResource: " + r.Slug + " needs a label, a view permission and at least one field")
	case r.Export == nil && r.Import == nil:
		panic("sdk.RegisterDataResource: " + r.Slug + " neither imports nor exports")
	case r.Import != nil && (r.ImportPermission == "" || !identified):
		panic("sdk.RegisterDataResource: importing " + r.Slug + " needs an import permission and an Identifier field to upsert on")
	}
	seen := map[string]bool{}
	for _, f := range r.Fields {
		if !dataColumn.MatchString(f.Name) || seen[f.Name] {
			panic("sdk.RegisterDataResource: " + r.Slug + " has a missing, repeated or not lowercase column " + f.Name)
		}
		if f.Type != "" && !slices.Contains(DataFieldTypes(), f.Type) {
			panic("sdk.RegisterDataResource: " + r.Slug + "'s column " + f.Name + " has an unknown type " + string(f.Type))
		}
		seen[f.Name] = true
	}
	for _, o := range r.Options {
		if o.Name == "" || o.Type == DataOptionSelect && len(o.Choices) == 0 {
			panic("sdk.RegisterDataResource: " + r.Slug + " has an option without a name, or a select without choices")
		}
	}
	dataResources.add(RegisteredDataResource{PluginSlug: pl.slug, Resource: cloneDataResource(r)},
		func(d RegisteredDataResource) bool { return d.PluginSlug == pl.slug && d.Resource.Slug == r.Slug },
		"sdk.RegisterDataResource: "+pl.slug+" already registered "+r.Slug)
}

func DataResources() []RegisteredDataResource {
	out := dataResources.all()
	for i := range out {
		out[i].Resource = cloneDataResource(out[i].Resource)
	}
	return out
}

// A copy sharing no slice with the plugin's own, so neither side can change the other's
func cloneDataResource(r DataResource) DataResource {
	r.Fields = slices.Clone(r.Fields)
	for i := range r.Fields {
		r.Fields[i].Enum = slices.Clone(r.Fields[i].Enum)
	}
	r.Options = slices.Clone(r.Options)
	for i := range r.Options {
		r.Options[i].Choices = slices.Clone(r.Options[i].Choices)
	}
	return r
}
