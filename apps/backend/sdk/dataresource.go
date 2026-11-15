package sdk

import (
	"context"
	"regexp"
	"slices"
	"sync"
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

// One decoded row - a malformed cell fails the row by itself, and Fail fails it with your own reason
type DataRow interface {
	Has(name string) bool
	Str(name string) string
	Int(name string) int64
	Float(name string) float64
	Bool(name string) bool
	Time(name string) *time.Time
	List(name string) []string
	Fail(format string, args ...any)
}

type DataOutcome int

const (
	DataSkipped DataOutcome = iota
	DataCreated
	DataUpdated
)

type DataRecord map[string]any

// Core owns the file: parsing, validation against the fields, dry runs, error isolation and progress. Import runs
// once per row, and an error fails only that row; Export streams records until it returns
type DataResource struct {
	Slug             string // lowercase, unique within the plugin - core files it as "<plugin>.<slug>"
	Label            string
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
	dataResourcesMu sync.Mutex
	dataResources   []RegisteredDataResource
	dataResourceKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	dataColumn      = regexp.MustCompile(`^[a-z0-9_:.-]+$`)
)

// Lists the resource in the dashboard's data hub - panics on a malformed one or a slug the plugin already used
func (pl *Plugin) RegisterDataResource(r DataResource) {
	identified := slices.ContainsFunc(r.Fields, func(f DataField) bool { return f.Identifier })
	switch {
	case !dataResourceKey.MatchString(r.Slug):
		panic("sdk.RegisterDataResource: slug " + r.Slug + " must be lowercase letters, digits and underscores, starting with a letter")
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
		seen[f.Name] = true
	}
	for _, o := range r.Options {
		if o.Name == "" || o.Type == DataOptionSelect && len(o.Choices) == 0 {
			panic("sdk.RegisterDataResource: " + r.Slug + " has an option without a name, or a select without choices")
		}
	}
	dataResourcesMu.Lock()
	defer dataResourcesMu.Unlock()
	if slices.ContainsFunc(dataResources, func(d RegisteredDataResource) bool { return d.PluginSlug == pl.slug && d.Resource.Slug == r.Slug }) {
		panic("sdk.RegisterDataResource: " + pl.slug + " already registered " + r.Slug)
	}
	dataResources = append(dataResources, RegisteredDataResource{PluginSlug: pl.slug, Resource: r})
}

func DataResources() []RegisteredDataResource {
	dataResourcesMu.Lock()
	defer dataResourcesMu.Unlock()
	return slices.Clone(dataResources)
}
