package dataio

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

func init() { registerCodec(jsonlCodec{}) }

const maxJSONLLineBytes = 8 << 20

var errNestedObject = errors.New("a nested object can't be a cell - flatten it into its own columns")

// One JSON object per line - what a pipeline wants when a spreadsheet isn't in the loop, and the only format that survives a value containing newlines untouched
type jsonlCodec struct{}

func (jsonlCodec) Format() Format      { return FormatJSONL }
func (jsonlCodec) Label() string       { return "JSON Lines" }
func (jsonlCodec) Extension() string   { return "jsonl" }
func (jsonlCodec) ContentType() string { return "application/x-ndjson" }

func (jsonlCodec) Writer(w io.Writer, cols []Field) (RowWriter, error) {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return &jsonlWriter{w: bufio.NewWriter(w), cols: names}, nil
}

type jsonlWriter struct {
	w    *bufio.Writer
	cols []string
}

// Empty cells are omitted rather than written as "" - a JSONL consumer reads a missing key as "not set", which is exactly the import semantics
func (w *jsonlWriter) Write(r *Row) error {
	obj := make(map[string]string, len(w.cols))
	for _, name := range w.cols {
		if v := r.values[name]; v != "" {
			obj[name] = v
		}
	}
	line, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if _, err := w.w.Write(line); err != nil {
		return err
	}
	return w.w.WriteByte('\n')
}

func (w *jsonlWriter) Close() error { return w.w.Flush() }

// The header is discovered from the first object, so a JSONL import must be homogeneous in its first line at least
func (jsonlCodec) Reader(r io.Reader) (RowReader, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxJSONLLineBytes) // a row with a long description blows the 64k default
	return &jsonlReader{sc: sc}, nil
}

type jsonlReader struct {
	sc      *bufio.Scanner
	headers []string
	line    int
}

func (r *jsonlReader) Header() []string { return r.headers }

func (r *jsonlReader) Next() (*Row, error) {
	for {
		if !r.sc.Scan() {
			if err := r.sc.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		r.line++
		raw := strings.TrimSpace(r.sc.Text())
		if raw == "" {
			continue
		}
		var obj map[string]any
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber() // a numeric SKU or a large minor-unit amount must not round through float64
		if err := dec.Decode(&obj); err != nil {
			return nil, fmt.Errorf("line %d: %w", r.line, err)
		}
		row := NewRow()
		row.Line = r.line
		for k, v := range obj {
			cell, err := jsonCell(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: %q: %w", r.line, k, err)
			}
			row.values[k] = strings.TrimSpace(cell)
			row.present[k] = true
			if !slices.Contains(r.headers, k) {
				r.headers = append(r.headers, k)
			}
		}
		return row, nil
	}
}

// An array is a list column, joined the way a CSV cell carries one - a nested object has no flat reading, so it's refused rather than stringified into noise
func jsonCell(v any) (string, error) {
	switch t := v.(type) {
	case map[string]any:
		return "", errNestedObject
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			switch e.(type) {
			case map[string]any, []any:
				return "", errNestedObject
			}
			parts = append(parts, stringify(e))
		}
		return strings.Join(parts, "|"), nil
	}
	return stringify(v), nil
}
