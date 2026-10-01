package dataio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

// Export streams the resource straight into w - nothing buffers the whole file, so an all-time
// export costs one row of memory rather than the catalog
func (reg *Registry) Export(ctx context.Context, run Run, w io.Writer) (ExportResult, error) {
	res, err := reg.Get(run.Resource)
	if err != nil {
		return ExportResult{}, err
	}
	if !res.CanExport() {
		return ExportResult{}, fmt.Errorf("%w: %s", ErrNotExportable, res.Slug)
	}
	codec, err := CodecFor(run.Format)
	if err != nil {
		return ExportResult{}, err
	}
	opts, err := resolveOpts(res, run.Opts, DirectionExport)
	if err != nil {
		return ExportResult{}, err
	}

	exp, err := res.NewExporter(ctx, opts)
	if err != nil {
		return ExportResult{}, err
	}
	extra, err := exp.Columns(ctx)
	if err != nil {
		return ExportResult{}, err
	}
	cols := columnsFor(res, extra)

	rw, err := codec.Writer(w, cols)
	if err != nil {
		return ExportResult{}, err
	}

	out := ExportResult{Columns: make([]string, len(cols))}
	for i, c := range cols {
		out.Columns[i] = c.Name
	}
	err = exp.Stream(ctx, func(row *Row) error {
		if err := rw.Write(row); err != nil {
			return err
		}
		out.Rows++
		if run.Progress != nil && out.Rows%progressEvery == 0 {
			run.Progress(out.Rows, 0)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if err := rw.Close(); err != nil {
		return out, err
	}
	if run.Progress != nil {
		run.Progress(out.Rows, out.Rows)
	}
	return out, nil
}

// Import decodes the source and hands each row to the resource. Rows are independent: one failing lands in
// Errors and the rest continue - a returned error means the FILE was unusable, not that some rows were
func (reg *Registry) Import(ctx context.Context, run Run, src io.Reader) (ImportResult, error) {
	res, err := reg.Get(run.Resource)
	if err != nil {
		return ImportResult{}, err
	}
	if !res.CanImport() {
		return ImportResult{}, fmt.Errorf("%w: %s", ErrNotImportable, res.Slug)
	}
	codec, err := CodecFor(run.Format)
	if err != nil {
		return ImportResult{}, err
	}
	opts, err := resolveOpts(res, run.Opts, DirectionImport)
	if err != nil {
		return ImportResult{}, err
	}

	rr, err := codec.Reader(src)
	if err != nil {
		return ImportResult{}, err
	}

	limit := res.MaxImportRows
	if limit <= 0 {
		limit = DefaultMaxImportRows
	}
	rows := make([]*Row, 0, 256)
	for {
		row, err := rr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ImportResult{}, fmt.Errorf("read row: %w", err)
		}
		rows = append(rows, row)
		if len(rows) > limit {
			return ImportResult{}, fmt.Errorf("more than %d rows - split the file", limit)
		}
	}
	if len(rows) == 0 {
		return ImportResult{}, fmt.Errorf("no data rows")
	}

	out := ImportResult{Total: len(rows)}
	// Header checks come after decoding because JSONL only knows its columns once a row is read
	header := make([]string, 0, len(rr.Header()))
	for _, h := range rr.Header() {
		header = append(header, res.canonical(h))
	}
	for _, row := range rows {
		row.renameAliases(res)
	}
	for _, id := range res.identifiers() {
		if !slices.Contains(header, id) {
			return ImportResult{}, fmt.Errorf("missing required column %q", id)
		}
	}
	for _, h := range header {
		if !res.knownColumn(h) {
			out.Warnings = append(out.Warnings, fmt.Sprintf("column %q is not recognised and was ignored", h))
		}
	}

	imp, err := res.NewImporter(ctx, opts)
	if err != nil {
		return ImportResult{}, err
	}

	// Every row carries the descriptor, which is what lets Enum and RequireDeclared read the declared set
	// instead of a list repeated at the call site
	index := newFieldSet(res.Fields)
	for _, row := range rows {
		row.fields = index
	}

	// Group by pass so everything a later pass depends on already exists - configurable parents before their variant children
	passer, _ := imp.(Passer)
	byPass := map[int][]*Row{}
	var passes []int
	for _, row := range rows {
		p := 0
		if passer != nil {
			p = passer.Pass(row)
		}
		if _, seen := byPass[p]; !seen {
			passes = append(passes, p)
		}
		byPass[p] = append(byPass[p], row)
	}
	sort.Ints(passes)

	done := 0
	var waiting []*Row
	for _, p := range passes {
		for _, row := range byPass[p] {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			if applyRow(ctx, imp, row, &out, true) != nil {
				waiting = append(waiting, row)
			}
			done++
			if run.Progress != nil && done%progressEvery == 0 {
				run.Progress(done, out.Total)
			}
		}
	}
	// Fixed point: each round must land at least one deferred row, so a dependency cycle ends instead of spinning
	for len(waiting) > 0 {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		var still []*Row
		for _, row := range waiting {
			if applyRow(ctx, imp, row, &out, true) != nil {
				still = append(still, row)
			}
		}
		if len(still) == len(waiting) {
			for _, row := range still {
				_ = applyRow(ctx, imp, row, &out, false)
			}
			break
		}
		waiting = still
	}

	if fin, ok := imp.(Finisher); ok {
		deferred, err := fin.Finish(ctx)
		if err != nil {
			return out, fmt.Errorf("finalise: %w", err)
		}
		for _, re := range deferred {
			if len(out.Errors) >= MaxRowErrors {
				out.Truncated = true
				break
			}
			re.Message = trimMessage(re.Message)
			out.Errors = append(out.Errors, re)
		}
	}
	if run.Progress != nil {
		run.Progress(out.Total, out.Total)
	}
	return out, nil
}

const progressEvery = 200

// A panicking row must not take the other 249,999 down with it - a deferred row's error comes back uncounted while canDefer holds
func applyRow(ctx context.Context, imp Importer, row *Row, out *ImportResult, canDefer bool) error {
	// Declared enums are checked before the handler, so a resource never sees an out-of-set value
	row.validateEnums()
	if err := row.Err(); err != nil {
		out.Failed++
		if len(out.Errors) < MaxRowErrors {
			out.Errors = append(out.Errors, RowError{Line: row.Line, Ref: row.Ref, Message: trimMessage(err.Error())})
		} else {
			out.Truncated = true
		}
		return nil
	}

	var (
		outcome Outcome
		err     error
	)
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				// No stack dump - a bad file would print 250k of them, and the row number is what actually locates it
				err = fmt.Errorf("panic: %v", rec)
			}
		}()
		outcome, err = imp.Row(ctx, row)
	}()

	// A handler that ignored a rejected cell still fails the row - Row.Err is the accumulated cell errors
	if err == nil {
		err = row.Err()
	}
	if err != nil && canDefer && isDeferred(err) {
		return err
	}
	if err != nil {
		out.Failed++
		if len(out.Errors) < MaxRowErrors {
			out.Errors = append(out.Errors, RowError{Line: row.Line, Ref: row.Ref, Message: trimMessage(err.Error())})
		} else {
			out.Truncated = true
		}
		return nil
	}
	switch outcome {
	case OutcomeCreated:
		out.Created++
	case OutcomeUpdated:
		out.Updated++
	default:
		out.Skipped++
	}
	return nil
}

// Postgres errors arrive with a paragraph of detail and hint appended, which is unreadable in a per-row table
func trimMessage(msg string) string {
	msg = strings.TrimSpace(strings.ReplaceAll(msg, "\n", " "))
	const max = 400
	if len(msg) > max {
		return msg[:max] + "..."
	}
	return msg
}

// Declared fields first in declaration order, then whatever the exporter discovered, deduped
func columnsFor(res Resource, extra []Field) []Field {
	cols := make([]Field, 0, len(res.Fields)+len(extra))
	seen := map[string]bool{}
	for _, f := range res.Fields {
		cols = append(cols, f)
		seen[f.Name] = true
	}
	for _, f := range extra {
		if !seen[f.Name] {
			cols = append(cols, f)
			seen[f.Name] = true
		}
	}
	return cols
}

// Template renders a header-only file so an admin never has to guess the columns
func (reg *Registry) Template(ctx context.Context, slug string, format Format, w io.Writer) error {
	res, err := reg.Get(slug)
	if err != nil {
		return err
	}
	codec, err := CodecFor(format)
	if err != nil {
		return err
	}
	cols := make([]Field, 0, len(res.Fields))
	for _, f := range res.Fields {
		if !f.ReadOnly {
			cols = append(cols, f)
		}
	}
	rw, err := codec.Writer(w, cols)
	if err != nil {
		return err
	}
	// One example row, since a bare header leaves the value shape to guesswork
	sample := NewRow()
	filled := false
	for _, f := range cols {
		if f.Example != "" {
			sample.Set(f.Name, f.Example)
			filled = true
		}
	}
	if filled {
		if err := rw.Write(sample); err != nil {
			return err
		}
	}
	return rw.Close()
}

// Checks a run's options without executing it, so a caller queueing work can reject a bad request
// immediately rather than handing back a job that is already doomed
func (reg *Registry) ValidateOptions(slug string, opts Opts, dir Direction) error {
	res, err := reg.Get(slug)
	if err != nil {
		return err
	}
	_, err = resolveOpts(res, opts, dir)
	return err
}

// Fills defaults and rejects a missing required option before any handler runs
func resolveOpts(res Resource, in Opts, dir Direction) (Opts, error) {
	values := in.Values()
	for _, def := range res.Options {
		if def.Applies != "" && def.Applies != dir {
			continue
		}
		v := strings.TrimSpace(values[def.Name])
		if v == "" && def.Default != "" {
			v = def.Default
		}
		if v == "" && def.Required {
			return Opts{}, fmt.Errorf("option %q is required", def.Name)
		}
		if v != "" && len(def.Choices) > 0 {
			ok := false
			for _, c := range def.Choices {
				if c.Value == v {
					ok = true
					break
				}
			}
			if !ok {
				return Opts{}, fmt.Errorf("option %q does not accept %q", def.Name, v)
			}
		}
		values[def.Name] = v
	}
	for name := range values {
		if _, ok := res.option(name); !ok {
			delete(values, name) // an option the resource never declared would silently do nothing
		}
	}
	out := in
	out.values = values
	return out, nil
}
