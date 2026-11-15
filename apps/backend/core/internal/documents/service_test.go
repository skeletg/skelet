package documents

import (
	"bytes"
	"context"
	"errors"
	"image"
	pngenc "image/png"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type fakeSettings map[string]string

func (f fakeSettings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f[path], nil
}

type fakeFiles map[string][]byte

func (fakeFiles) PublicURL(key string) string { return "https://cdn.example/media/" + key }
func (f fakeFiles) GetBytes(_ context.Context, key string) ([]byte, error) {
	if b, ok := f[key]; ok {
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

func newTestService(t *testing.T, s fakeSettings, plugin PluginRenderer) *Service {
	files := fakeFiles{"brand/logo one.png": smallPNG(t)}
	return NewService(s, files, Lookups{Channels: fakeChannels{}, Countries: fakeCountries{}}, plugin)
}

func TestTheChannelsSettingsDressTheDocument(t *testing.T) {
	ctx := context.Background()
	o := testOrder()
	plain := newTestService(t, fakeSettings{}, nil).Input(ctx, KindInvoice, o, nil)
	if plain.Store.Name != "Outlet" || len(plain.Store.Lines) != 1 || plain.Store.Lines[0] != "https://outlet.example" || plain.Store.Logo != nil {
		t.Errorf("nothing set: %+v - want the channel's name and address", plain.Store)
	}

	s := newTestService(t, fakeSettings{
		channel.ConfigStoreName:          "Skelet Apparel",
		sellerPath:                       "Skelet Apparel GmbH\n\n  Hauptstrasse 1, 10115 Berlin \nUSt-IdNr. DE123456789",
		"sales/documents/invoice_footer": "Payable within 14 days.",
		brandingLogo:                     "https://cdn.example/media/brand/logo%20one.png",
	}, nil)
	in := s.Input(ctx, KindInvoice, o, nil)
	if in.Store.Name != "Skelet Apparel" || len(in.Store.Lines) != 3 || in.Store.Lines[1] != "Hauptstrasse 1, 10115 Berlin" {
		t.Errorf("letterhead %+v", in.Store)
	}
	if in.Footer != "Payable within 14 days." || in.Note != "Note for GB" || in.Store.Logo == nil {
		t.Errorf("footer %q, note %q, logo %d bytes", in.Footer, in.Note, len(in.Store.Logo))
	}
	if slip := s.Input(ctx, KindPackingSlip, o, nil); slip.Note != "" || slip.Footer != "" {
		t.Errorf("a slip carries no tax note and its own footer: %q / %q", slip.Note, slip.Footer)
	}
	elsewhere := newTestService(t, fakeSettings{logoPath: "https://elsewhere.example/logo.png"}, nil)
	if logo := elsewhere.Input(ctx, KindInvoice, o, nil).Store.Logo; logo != nil {
		t.Error("a logo hosted elsewhere was fetched")
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
		pdf, err := newTestService(t, fakeSettings{}, tc.plugin).Document(ctx, KindInvoice, testOrder(), nil)
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
