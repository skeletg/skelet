package plugin

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestAPluginDataResourceRunsThroughTheDataHub(t *testing.T) {
	balances := map[string]int64{"ada@example.com": 10}
	var dryRuns []bool
	sdk.NewPluginForTest("data-test").RegisterDataResource(sdk.DataResource{
		Slug: "points", Label: "Points", ViewPermission: "data_test.view", ImportPermission: "data_test.edit",
		Fields: []sdk.DataField{
			{Name: "email", Identifier: true, Required: true},
			{Name: "points", Type: sdk.DataInteger},
			{Name: "since", Type: sdk.DataDate, ReadOnly: true},
		},
		Export: func(_ context.Context, _ sdk.DataRun, emit func(sdk.DataRecord) error) error {
			return emit(sdk.DataRecord{"email": "ada@example.com", "points": balances["ada@example.com"], "since": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
		},
		Import: func(_ context.Context, run sdk.DataRun, row sdk.DataRow) (sdk.DataOutcome, error) {
			dryRuns = append(dryRuns, run.DryRun)
			if strings.HasSuffix(row.Str("email"), "@blocked.example") {
				row.Fail("blocked addresses earn nothing")
				return sdk.DataSkipped, nil
			}
			_, known := balances[row.Str("email")]
			if !run.DryRun {
				balances[row.Str("email")] = row.Int("points")
			}
			if known {
				return sdk.DataUpdated, nil
			}
			return sdk.DataCreated, nil
		},
	})
	reg := dataio.NewRegistry()
	for _, res := range DataResources() {
		reg.Register(res)
	}
	ctx := context.Background()

	var out bytes.Buffer
	if _, err := reg.Export(ctx, dataio.Run{Resource: "data-test.points", Format: dataio.FormatCSV, Opts: dataio.NewOpts(nil)}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "email,points,since") || !strings.Contains(out.String(), "ada@example.com,10,2026-01-02") {
		t.Errorf("export:\n%s", out.String())
	}

	file := "email,points\nada@example.com,25\ngrace@example.com,5\nnot-a-number@example.com,lots\nx@blocked.example,3\n"
	dry := dataio.NewOpts(nil)
	dry.DryRun = true
	res, err := reg.Import(ctx, dataio.Run{Resource: "data-test.points", Format: dataio.FormatCSV, Opts: dry}, strings.NewReader(file))
	if err != nil || res.Updated != 1 || res.Created != 1 || res.Failed != 2 || balances["ada@example.com"] != 10 {
		t.Fatalf("dry run %+v, %v, balance %d", res, err, balances["ada@example.com"])
	}
	if res, err = reg.Import(ctx, dataio.Run{Resource: "data-test.points", Format: dataio.FormatCSV, Opts: dataio.NewOpts(nil)}, strings.NewReader(file)); err != nil || balances["ada@example.com"] != 25 || balances["grace@example.com"] != 5 {
		t.Fatalf("import %+v, %v, balances %v", res, err, balances)
	}
	if len(dryRuns) == 0 || !dryRuns[0] || dryRuns[len(dryRuns)-1] {
		t.Errorf("the plugin saw dry runs %v", dryRuns)
	}
}
