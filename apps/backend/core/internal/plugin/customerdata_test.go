package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A plugin's kept data comes out with the export and goes with an erasure - every plugin asked even when one fails, and
// a panic one plugin's erasure can't take the rest down
func TestCustomerDataLeavesWithTheCustomer(t *testing.T) {
	sdk.ResetForTest()
	t.Cleanup(sdk.ResetForTest)
	who := uuid.New()
	var erased []string
	acme, other := sdk.NewPluginForTest("acme"), sdk.NewPluginForTest("other")
	acme.RegisterCustomerData(sdk.CustomerData{
		Key: "warranties",
		Export: func(_ context.Context, s sdk.CustomerDataSubject) (any, error) {
			return []string{"warranty for " + s.Email}, nil
		},
		Erase: func(_ context.Context, s sdk.CustomerDataSubject) error {
			panic("erase exploded")
		},
	})
	other.RegisterCustomerData(sdk.CustomerData{
		Key:    "notes",
		Export: func(context.Context, sdk.CustomerDataSubject) (any, error) { return nil, nil },
		Erase: func(_ context.Context, s sdk.CustomerDataSubject) error {
			erased = append(erased, s.CustomerID.String())
			return nil
		},
	})

	got, err := CustomerDataExport(context.Background(), who, uuid.New(), "ana@example.com")
	if err != nil || len(got) != 1 || got["acme:warranties"].([]string)[0] != "warranty for ana@example.com" {
		t.Fatalf("export %v, %v - want acme's section alone, other's nil left out", got, err)
	}

	err = EraseCustomerData(context.Background(), who, uuid.New(), "ana@example.com")
	if err == nil || !strings.Contains(err.Error(), "acme: erase warranties") || len(erased) != 1 || erased[0] != who.String() {
		t.Errorf("erase %v, erased %v - want acme's panic reported and other's erase run", err, erased)
	}

	failing := sdk.NewPluginForTest("failing")
	failing.RegisterCustomerData(sdk.CustomerData{Key: "rows", Export: func(context.Context, sdk.CustomerDataSubject) (any, error) {
		return nil, errors.New("db down")
	}})
	if _, err := CustomerDataExport(context.Background(), who, uuid.New(), ""); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Errorf("a failed section: %v - want the export failed", err)
	}
}
