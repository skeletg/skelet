package sdk

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterDocumentRenderer(t *testing.T) {
	t.Cleanup(func() { documentRenderers = nil })
	pdf := func(context.Context, DocumentRequest) ([]byte, error) { return []byte("%PDF-1.7"), nil }
	acme := &Plugin{slug: "acme"}
	acme.RegisterDocumentRenderer(DocumentInvoice, pdf)
	acme.RegisterDocumentRenderer(DocumentPackingSlip, pdf)
	(&Plugin{slug: "other"}).RegisterDocumentRenderer(DocumentInvoice, pdf)
	if got := DocumentRenderers(); len(got) != 3 || got[0].PluginSlug != "acme" || got[2].Kind != DocumentInvoice {
		t.Fatalf("registered %+v", got)
	}
	for name, register := range map[string]func(){
		"a second invoice": func() { acme.RegisterDocumentRenderer(DocumentInvoice, pdf) },
		"an unknown kind":  func() { acme.RegisterDocumentRenderer("receipt", pdf) },
		"nil":              func() { acme.RegisterDocumentRenderer(DocumentCreditMemo, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r, _ := recover().(string); !strings.HasPrefix(r, "sdk.RegisterDocumentRenderer") {
					t.Errorf("panic %q", r)
				}
			}()
			register()
		})
	}
}
