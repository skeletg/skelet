package plugin

import (
	"context"
	"database/sql"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/datapatch"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Verifies a plugin's SDK data-patch registration lands in the core datapatch registry with its identity intact, and that the patch's *sql.Tx is forwarded to the plugin's Apply
// The real DB-touching demonstration lives in the plugins/example-data-patch plugin
func TestRegisterDataPatch_Bridge(t *testing.T) {
	const id = "bridge-test:0001"
	called := false
	var gotTx *sql.Tx

	h := &Host{}
	if err := h.RegisterDataPatch("dp-test", sdk.DataPatchRegistration{
		Namespace: "bridge-test",
		ID:        id,
		DependsOn: []string{"bridge-test:0000"},
		Apply: func(_ context.Context, tx *sql.Tx) error {
			called = true
			gotTx = tx
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterDataPatch: %v", err)
	}

	var patch *datapatch.Patch
	for _, p := range datapatch.Registered() {
		if p.ID == id {
			pp := p
			patch = &pp
			break
		}
	}
	if patch == nil {
		t.Fatal("plugin patch did not reach the datapatch registry")
	}
	if patch.Namespace != "bridge-test" {
		t.Errorf("namespace: got %q, want %q", patch.Namespace, "bridge-test")
	}
	if len(patch.DependsOn) != 1 || patch.DependsOn[0] != "bridge-test:0000" {
		t.Errorf("DependsOn not forwarded: %v", patch.DependsOn)
	}

	// The runner hands the patch a Context and the bridge must forward its Tx to the plugin's Apply - use a sentinel tx pointer
	sentinel := &sql.Tx{}
	if err := patch.Apply(context.Background(), &datapatch.Context{Tx: sentinel}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !called {
		t.Error("plugin Apply was not invoked through the bridge")
	}
	if gotTx != sentinel {
		t.Error("bridge did not forward the *sql.Tx to the plugin Apply")
	}
}

func TestAPluginPatchWithoutANamespaceIsFiledUnderItsPlugin(t *testing.T) {
	const id = "bridge-test:no-namespace"
	if err := (&Host{}).RegisterDataPatch("dp-nons", sdk.DataPatchRegistration{
		ID:    id,
		Apply: func(context.Context, *sql.Tx) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	for _, p := range datapatch.Registered() {
		if p.ID == id {
			if p.Namespace != "dp-nons" {
				t.Fatalf("namespace %q, want the plugin's slug", p.Namespace)
			}
			return
		}
	}
	t.Fatal("plugin patch did not reach the datapatch registry")
}
