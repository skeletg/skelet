package documents

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	pngenc "image/png"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type fakeSettings map[string]string

func (f fakeSettings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	if path == "broken" || f["broken"] == path {
		return "", errors.New("db down")
	}
	return f[path], nil
}

type fakeFiles struct {
	m     map[string][]byte
	reads int
}

func (*fakeFiles) PublicURL(key string) string { return "https://cdn.example/media/" + key }
func (f *fakeFiles) Head(_ context.Context, key string) (int64, string, bool, error) {
	b, ok := f.m[key]
	return int64(len(b)), "image/png", ok, nil
}
func (f *fakeFiles) GetBytes(_ context.Context, key string) ([]byte, error) {
	f.reads++
	if b, ok := f.m[key]; ok {
		return b, nil
	}
	return nil, errors.New("missing")
}

type fakeChannels struct{}

func (fakeChannels) GetByID(context.Context, uuid.UUID) (channel.Channel, error) {
	return channel.Channel{Name: "Outlet", PublicURL: "https://outlet.example"}, nil
}

type fakeCountries struct{}

func (fakeCountries) GetByCode(_ context.Context, code string) (country.Country, error) {
	return country.Country{Code: code, InvoiceNote: "Note for " + code}, nil
}

func smallPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := pngenc.Encode(&b, image.NewRGBA(image.Rect(0, 0, 40, 20))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func newTestService(t *testing.T, s fakeSettings, plugin PluginRenderer) (*Service, *fakeFiles) {
	files := &fakeFiles{m: map[string][]byte{"brand/logo one.png": smallPNG(t)}}
	return NewService(s, files, Lookups{Channels: fakeChannels{}, Countries: fakeCountries{}}, plugin), files
}

func input(t *testing.T, s *Service, kind Kind, o order.Order) Input {
	t.Helper()
	in, err := s.Input(context.Background(), kind, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestTheChannelsSettingsDressTheDocument(t *testing.T) {
	o := testOrder()
	o.BusinessIDs = []order.BusinessID{{Label: "VAT number", Value: "GB123"}}
	plainSvc, _ := newTestService(t, fakeSettings{}, nil)
	if plain := input(t, plainSvc, KindInvoice, o); plain.Store.Name != "Outlet" || len(plain.Store.Lines) != 1 || plain.Store.Lines[0] != "https://outlet.example" || plain.Store.Logo != nil {
		t.Errorf("nothing set: %+v - want the channel's name and address", plain.Store)
	}

	s, files := newTestService(t, fakeSettings{
		channel.ConfigStoreName:          "Skelet Apparel",
		sellerPath:                       "Skelet Apparel GmbH\n\n  Hauptstrasse 1, 10115 Berlin \nUSt-IdNr. DE123456789",
		"sales/documents/invoice_footer": "Payable within 14 days.",
		channel.ConfigLogoURL:            "https://cdn.example/media/brand/logo%20one.png",
	}, nil)
	in := input(t, s, KindInvoice, o)
	if in.Store.Name != "Skelet Apparel" || len(in.Store.Lines) != 3 || in.Store.Lines[1] != "Hauptstrasse 1, 10115 Berlin" {
		t.Errorf("letterhead %+v", in.Store)
	}
	if in.Footer != "Payable within 14 days." || in.Note != "Note for GB" || in.Store.Logo == nil {
		t.Errorf("footer %q, note %q, logo %d bytes", in.Footer, in.Note, len(in.Store.Logo))
	}
	if slip := input(t, s, KindPackingSlip, o); slip.Note != "" || slip.Footer != "" {
		t.Errorf("a slip carries no tax note and its own footer: %q / %q", slip.Note, slip.Footer)
	}
	if files.reads != 1 {
		t.Errorf("the logo was read %d times for three documents", files.reads)
	}
	personal := testOrder()
	if in := input(t, s, KindInvoice, personal); in.Note != "" {
		t.Errorf("a personal purchase carries the country's business note: %q", in.Note)
	}
	personal.Seller = &order.Seller{Name: "Skelet Apparel", Lines: []string{"Old Street 9, 10115 Berlin"}}
	if in := input(t, s, KindInvoice, personal); len(in.Store.Lines) != 1 || in.Store.Lines[0] != "Old Street 9, 10115 Berlin" {
		t.Errorf("an order placed before the address changed: %+v", in.Store)
	}
	elsewhere, _ := newTestService(t, fakeSettings{logoPath: "https://elsewhere.example/logo.png"}, nil)
	if logo := input(t, elsewhere, KindInvoice, o).Store.Logo; logo != nil {
		t.Error("a logo hosted elsewhere was fetched")
	}
	broken, _ := newTestService(t, fakeSettings{"broken": sellerPath}, nil)
	if _, err := broken.Input(context.Background(), KindInvoice, testOrder(), nil); err == nil {
		t.Error("an invoice went out without its seller's details")
	}
}

// A PNG whose header alone claims a size - all a decompression bomb needs
func claimedPNG(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = 8
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(ihdr)))
	chunk = append(chunk, "IHDR"...)
	chunk = append(chunk, ihdr...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	return append([]byte("\x89PNG\r\n\x1a\n"), chunk...)
}

func TestALogoClaimingAHugeSizeIsNeverDecoded(t *testing.T) {
	if _, _, ok := logoPNG(claimedPNG(100_000, 100_000)); ok {
		t.Error("a logo claiming ten billion pixels was decoded")
	}
	if _, _, ok := logoPNG(smallPNG(t)); !ok {
		t.Error("an ordinary logo was refused")
	}
}

func TestAPluginMakesTheDocument(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		plugin PluginRenderer
		want   string
	}{
		"none on": {func(context.Context, Input, func() ([]byte, error)) ([]byte, bool, error) { return nil, false, nil }, "core"},
		"its own PDF": {func(context.Context, Input, func() ([]byte, error)) ([]byte, bool, error) {
			return []byte("%PDF-1.7 plugin"), true, nil
		}, "plugin"},
		"core's, wrapped": {func(_ context.Context, _ Input, core func() ([]byte, error)) ([]byte, bool, error) {
			pdf, err := core()
			return pdf, true, err
		}, "core"},
		"not a PDF": {func(context.Context, Input, func() ([]byte, error)) ([]byte, bool, error) {
			return []byte("<html>"), true, nil
		}, "error"},
		"a failure": {func(context.Context, Input, func() ([]byte, error)) ([]byte, bool, error) {
			return nil, true, errors.New("down")
		}, "error"},
	} {
		svc, _ := newTestService(t, fakeSettings{}, tc.plugin)
		pdf, err := svc.Document(ctx, KindInvoice, testOrder(), nil)
		got := "core"
		switch {
		case err != nil:
			got = "error"
		case strings.Contains(string(pdf), "plugin"):
			got = "plugin"
		case !bytes.HasPrefix(pdf, []byte("%PDF-")):
			got = "garbage"
		}
		if got != tc.want {
			t.Errorf("%s: got %s (%v), want %s", name, got, err, tc.want)
		}
	}
}

func TestAnIssuedInvoiceReadsAsItWasPlaced(t *testing.T) {
	s, _ := newTestService(t, fakeSettings{"sales/documents/invoice_footer": "New bank details"}, nil)
	o := testOrder()
	o.Seller = &order.Seller{Name: "Skelet Apparel", Lines: []string{"Old Street 9"}, Footers: map[string]string{"invoice": "Old bank details"}, Note: "Reverse charge"}
	if in := input(t, s, KindInvoice, o); in.Footer != "Old bank details" || in.Note != "Reverse charge" {
		t.Errorf("an invoice re-printed after the terms changed: %q / %q", in.Footer, in.Note)
	}
	if slip := input(t, s, KindPackingSlip, o); slip.Note != "" {
		t.Errorf("a packing slip carries the tax note: %q", slip.Note)
	}
}
