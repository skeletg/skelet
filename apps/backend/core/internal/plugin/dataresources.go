package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

var _ sdk.DataRow = (*dataio.Row)(nil)

func DataResources() []dataio.Resource {
	regs := sdk.DataResources()
	out := make([]dataio.Resource, len(regs))
	for i, r := range regs {
		out[i] = dataResource(r)
	}
	return out
}

func dataResource(reg sdk.RegisteredDataResource) dataio.Resource {
	d, slug := reg.Resource, reg.PluginSlug
	res := dataio.Resource{
		Slug: slug + "." + d.Slug, Label: d.Label, Group: "Plugins", Description: d.Description,
		ViewPermission: d.ViewPermission, ImportPermission: d.ImportPermission, RequireGlobalPermission: d.Global,
	}
	for _, f := range d.Fields {
		res.Fields = append(res.Fields, dataio.Field{
			Name: f.Name, Label: f.Label, Description: f.Description, Example: f.Example, Type: fieldType(f.Type),
			Required: f.Required, Identifier: f.Identifier, ReadOnly: f.ReadOnly, Enum: f.Enum,
		})
	}
	for _, o := range d.Options {
		opt := dataio.OptionDef{Name: o.Name, Label: o.Label, Description: o.Description, Type: dataio.OptionType(o.Type), Required: o.Required, Default: o.Default}
		for _, c := range o.Choices {
			opt.Choices = append(opt.Choices, dataio.Choice{Value: c.Value, Label: c.Label})
		}
		res.Options = append(res.Options, opt)
	}
	if d.Export != nil {
		res.NewExporter = func(ctx context.Context, o dataio.Opts) (dataio.Exporter, error) {
			if !Enabled(ctx, slug, o.ChannelID) {
				return nil, pluginOff(slug)
			}
			return pluginExporter{slug: slug, run: dataRun(o), export: d.Export, fields: d.Fields}, nil
		}
	}
	if d.Import != nil {
		res.NewImporter = func(ctx context.Context, o dataio.Opts) (dataio.Importer, error) {
			if !Enabled(ctx, slug, o.ChannelID) {
				return nil, pluginOff(slug)
			}
			return pluginImporter{slug: slug, run: dataRun(o), imp: d.Import, fields: d.Fields}, nil
		}
	}
	return res
}

func fieldType(t sdk.DataFieldType) dataio.FieldType {
	if t == "" {
		return dataio.FieldText
	}
	return dataio.FieldType(t)
}

func pluginOff(slug string) error {
	return fmt.Errorf("the %s plugin is switched off for this channel", slug)
}

func dataRun(o dataio.Opts) sdk.DataRun {
	return sdk.DataRun{ChannelID: o.ChannelID, Locale: o.Locale, DryRun: o.DryRun, Options: o.Values()}
}

type pluginExporter struct {
	slug   string
	run    sdk.DataRun
	export func(context.Context, sdk.DataRun, func(sdk.DataRecord) error) error
	fields []sdk.DataField
}

func (pluginExporter) Columns(context.Context) ([]dataio.Field, error) { return nil, nil }

// Instrumented for attribution, metrics and panics, but never timed or metered - an export runs as long as its data
func (e pluginExporter) Stream(ctx context.Context, emit func(*dataio.Row) error) (err error) {
	ctx, finish, gate := Instrument(ctx, e.slug, "data export", e.run.ChannelID, Unmetered(), Untimed())
	defer finish(&err)
	if gate != nil {
		return gate
	}
	return translateUserError(e.export(ctx, e.run, func(rec sdk.DataRecord) error {
		row := dataio.NewRow()
		for name, v := range rec {
			if _, isText := v.(string); !isText && e.isJSON(name) {
				b, err := json.Marshal(v)
				if err != nil {
					return fmt.Errorf("the %s column isn't JSON: %w", name, err)
				}
				v = string(b)
			}
			row.Set(name, v)
		}
		return emit(row)
	}))
}

func (e pluginExporter) isJSON(name string) bool {
	return slices.ContainsFunc(e.fields, func(f sdk.DataField) bool { return f.Name == name && f.Type == sdk.DataJSON })
}

type pluginImporter struct {
	slug   string
	run    sdk.DataRun
	imp    func(context.Context, sdk.DataRun, sdk.DataRow) (sdk.DataOutcome, error)
	fields []sdk.DataField
}

// Every typed cell is parsed before the plugin sees the row, so a bad one fails it on a dry run too, whichever cells
// the plugin's own code reads - all but read-only ones, which an import ignores, so a spreadsheet's reformatting of
// them never fails a row
func (i pluginImporter) Row(ctx context.Context, r *dataio.Row) (out dataio.Outcome, err error) {
	for _, f := range i.fields {
		if f.ReadOnly {
			continue
		}
		switch f.Type {
		case sdk.DataInteger, sdk.DataMoney:
			r.Int(f.Name)
		case sdk.DataNumber:
			r.Float(f.Name)
		case sdk.DataBool:
			r.Bool(f.Name)
		case sdk.DataDate:
			r.Time(f.Name)
		case sdk.DataJSON:
			var v any
			r.JSON(f.Name, &v)
		}
	}
	if err := r.Err(); err != nil {
		return dataio.OutcomeSkipped, err
	}
	ctx, finish, gate := Instrument(ctx, i.slug, "data import", i.run.ChannelID, Unmetered(), Untimed())
	defer finish(&err)
	if gate != nil {
		return dataio.OutcomeSkipped, gate
	}
	got, err := i.imp(ctx, i.run, r)
	if err != nil {
		return dataio.OutcomeSkipped, translateUserError(err)
	}
	switch got {
	case sdk.DataCreated:
		return dataio.OutcomeCreated, nil
	case sdk.DataUpdated:
		return dataio.OutcomeUpdated, nil
	}
	return dataio.OutcomeSkipped, nil
}
