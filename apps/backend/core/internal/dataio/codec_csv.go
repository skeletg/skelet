package dataio

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

func init() { registerCodec(csvCodec{}) }

type csvCodec struct{}

func (csvCodec) Format() Format      { return FormatCSV }
func (csvCodec) Label() string       { return "CSV" }
func (csvCodec) Extension() string   { return "csv" }
func (csvCodec) ContentType() string { return "text/csv; charset=utf-8" }

func (csvCodec) Writer(w io.Writer, cols []Field) (RowWriter, error) {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(names); err != nil {
		return nil, err
	}
	guard := make([]bool, len(cols))
	for i, c := range cols {
		guard[i] = !numericField(c.Type)
	}
	return &csvWriter{cw: cw, cols: names, guard: guard, rec: make([]string, len(names))}, nil
}

type csvWriter struct {
	cw    *csv.Writer
	cols  []string
	guard []bool
	rec   []string
}

func (w *csvWriter) Write(r *Row) error {
	for i, name := range w.cols {
		v := r.values[name]
		if w.guard[i] && needsQuote(v) {
			v = formulaQuote + v
		}
		w.rec[i] = v
	}
	return w.cw.Write(w.rec)
}

// A spreadsheet runs a cell starting with one of these as a formula, so text from shoppers and admins is quoted on the way out and unquoted on the way back in - numeric columns are left alone so a negative amount stays a number
const formulaQuote = "'"

// A value already led by quotes before a formula character gets one more, so the import's single strip is exact
func needsQuote(v string) bool {
	return formulaLike(strings.TrimLeft(v, formulaQuote))
}

func formulaLike(v string) bool {
	return v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0]))
}

func numericField(t FieldType) bool {
	switch t {
	case FieldInteger, FieldNumber, FieldMoney, FieldBool, FieldDate:
		return true
	}
	return false
}

func (w *csvWriter) Close() error {
	w.cw.Flush()
	return w.cw.Error()
}

// Streams rather than ReadAll - a 250k-row import shouldn't sit in memory twice
func (csvCodec) Reader(r io.Reader) (RowReader, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // ragged rows map by header instead of erroring
	cr.TrimLeadingSpace = true
	rec, err := cr.Read()
	if errors.Is(err, io.EOF) { // a wrapping reader (size cap, gzip) can hand back a wrapped EOF, which == would miss and report as a read failure

		return nil, fmt.Errorf("the file is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	headers := make([]string, len(rec))
	seen := map[string]bool{}
	for i, h := range rec {
		h = strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")) // spreadsheet exports lead with a BOM, and a literal one mid-file won't compile
		if h == "" {
			continue
		}
		if seen[h] {
			return nil, fmt.Errorf("duplicate column %q", h)
		}
		seen[h] = true
		headers[i] = h
	}
	return &csvReader{cr: cr, headers: headers, line: 1}, nil
}

type csvReader struct {
	cr      *csv.Reader
	headers []string
	line    int
}

func (r *csvReader) Header() []string {
	out := make([]string, 0, len(r.headers))
	for _, h := range r.headers {
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

// Wholly blank rows are skipped - a trailing newline is not a record
func (r *csvReader) Next() (*Row, error) {
	for {
		rec, err := r.cr.Read()
		if err != nil {
			return nil, err
		}
		r.line++
		row := NewRow()
		row.Line = r.line
		blank := true
		for i, h := range r.headers {
			if h == "" {
				continue
			}
			v := ""
			if i < len(rec) {
				v = strings.TrimSpace(rec[i])
				if rest, ok := strings.CutPrefix(v, formulaQuote); ok && needsQuote(rest) {
					v = rest
				}
			}
			row.values[h] = v
			row.present[h] = true
			if v != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		return row, nil
	}
}
