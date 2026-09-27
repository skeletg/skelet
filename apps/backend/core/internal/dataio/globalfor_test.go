package dataio

import "testing"

func TestAGlobalImportResourceStillExportsPerChannel(t *testing.T) {
	r := Resource{RequireGlobalImport: true}
	if !r.GlobalFor(DirectionImport) {
		t.Error("an import that rewrites shared rows was gated per channel")
	}
	if r.GlobalFor(DirectionExport) {
		t.Error("the export of a globally-imported resource demanded the global grant")
	}
	if !(Resource{RequireGlobalPermission: true}).GlobalFor(DirectionExport) {
		t.Error("RequireGlobalPermission stopped covering exports")
	}
}
