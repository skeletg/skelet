package plugin

import (
	"context"
	"fmt"

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
		ViewPermission: d.ViewPermission, ImportPermission: d.ImportPermission,
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
			return pluginExporter{run: dataRun(o), export: d.Export}, nil
		}
	}
	if d.Import != nil {
		res.NewImporter = func(ctx context.Context, o dataio.Opts) (dataio.Importer, error) {
			if !Enabled(ctx, slug, o.ChannelID) {
				return nil, pluginOff(slug)
			}
			return pluginImporter{run: dataRun(o), imp: d.Import, fields: d.Fields}, nil
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
	run    sdk.DataRun
	export func(context.Context, sdk.DataRun, func(sdk.DataRecord) error) error
}

func (pluginExporter) Columns(context.Context) ([]dataio.Field, error) { return nil, nil }

func (e pluginExporter) Stream(ctx context.Context, emit func(*dataio.Row) error) error {
	return translateUserError(e.export(ctx, e.run, func(rec sdk.DataRecord) error {
		row := dataio.NewRow()
		for name, v := range rec {
			row.Set(name, v)
		}
		return emit(row)
	}))
}

type pluginImporter struct {
	run    sdk.DataRun
	imp    func(context.Context, sdk.DataRun, sdk.DataRow) (sdk.DataOutcome, error)
	fields []sdk.DataField
}

// Every typed cell is parsed before the plugin sees the row, so a bad one fails it on a dry run too, whichever cells
// the plugin's own code reads
func (i pluginImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	for _, f := range i.fields {
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
	out, err := i.imp(ctx, i.run, r)
	if err != nil {
		return dataio.OutcomeSkipped, translateUserError(err)
	}
	switch out {
	case sdk.DataCreated:
		return dataio.OutcomeCreated, nil
	case sdk.DataUpdated:
		return dataio.OutcomeUpdated, nil
	}
	return dataio.OutcomeSkipped, nil
}
