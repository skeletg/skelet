package dataio

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

type Format string

const (
	FormatCSV   Format = "csv"
	FormatJSONL Format = "jsonl"
)

type RowWriter interface {
	Write(*Row) error
	Close() error
}

// Next returns io.EOF when the source is spent
type RowReader interface {
	Header() []string
	Next() (*Row, error)
}

// A wire format. Adding one is this interface plus a registerCodec call - nothing in the resources or the driver changes
type Codec interface {
	Format() Format
	Label() string
	Extension() string
	ContentType() string
	Writer(w io.Writer, cols []Field) (RowWriter, error)
	Reader(r io.Reader) (RowReader, error)
}

var (
	codecMu sync.RWMutex
	codecs  = map[Format]Codec{}
)

func registerCodec(c Codec) {
	codecMu.Lock()
	defer codecMu.Unlock()
	if _, dup := codecs[c.Format()]; dup {
		panic(fmt.Sprintf("dataio: format %q already registered", c.Format()))
	}
	codecs[c.Format()] = c
}

func CodecFor(f Format) (Codec, error) {
	codecMu.RLock()
	defer codecMu.RUnlock()
	c, ok := codecs[f]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownFormat, f)
	}
	return c, nil
}

func Formats() []Codec {
	codecMu.RLock()
	defer codecMu.RUnlock()
	out := make([]Codec, 0, len(codecs))
	for _, c := range codecs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Format() < out[j].Format() })
	return out
}
