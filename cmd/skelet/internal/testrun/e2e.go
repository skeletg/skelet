package testrun

import (
	"fmt"
	"os"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const E2EAbout = `The money-path e2e suite, against its own disposable databases - skelet_e2e and skelet_e2e_signals,
and a ClickHouse spine of the same name - since it writes real orders and never cleans up. Fresh each
run, because a kept database grows: --keep reuses it. A Signals process for those databases runs beside
the scenarios, since core reaches the suite over the contract even in a test, and it's stopped however
the run ends. Needs the full stack up - not --lean, which leaves ClickHouse out. Any other flag goes
to go test.`

const (
	e2eDB    = "skelet_e2e"
	e2eSigDB = "skelet_e2e_signals"
	e2eDBURL = "postgres://skelet:skelet@db:5432/" + e2eDB + "?sslmode=disable"
	e2eSigDS = "postgres://skelet:skelet@signals-db:5432/" + e2eSigDB + "?sslmode=disable"
	e2eCHURL = "clickhouse://skelet:skelet@clickhouse:9000/" + e2eSigDB
	// Ports the dev stack's own Signals process doesn't use
	e2eGRPC = "18092"
	e2eHTTP = "18090"
	e2eBin  = "/tmp/skelet-e2e-signals"
)

func E2E(root string, args []string) int {
	fresh := true
	if len(args) > 0 && (args[0] == "--keep" || args[0] == "--fresh") {
		fresh = args[0] != "--keep"
		args = args[1:]
	}
	lib := compose.Load(root)
	// dbmate, the Signals process and go test all run inside the app container
	if !lib.Running("app") {
		fmt.Fprintln(os.Stderr, "dev stack not running - start it with skelet up")
		return 1
	}
	if !lib.Running("clickhouse") {
		fmt.Fprintln(os.Stderr, "the e2e suite needs ClickHouse for its spine - start the full stack with skelet up, not --lean")
		return 1
	}
	sys.HandleInterrupts()
	if lock("test-e2e", root) != nil {
		return 1
	}
	d := dbs{root: root, lib: lib}
	exec := func(argv ...string) error {
		cmd := compose.Command(root, lib.Env(), argv...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return sys.Run(cmd)
	}

	fmt.Println("==> preparing disposable e2e databases")
	for _, step := range []func() error{
		func() error { return d.postgres("db", e2eDB, fresh) },
		func() error { return d.postgres("signals-db", e2eSigDB, fresh) },
		func() error { return d.clickhouseDB(e2eSigDB, fresh) },
	} {
		if err := step(); err != nil {
			return fail(err)
		}
	}
	d.unmarkDev()
	for _, m := range []struct{ label, url, dir string }{
		{"commerce (" + e2eDB + ")", e2eDBURL, "internal/db/migrations"},
		{"signals (" + e2eSigDB + ")", e2eSigDS, signals + "/internal/db/migrations"},
		{"the spine (clickhouse " + e2eSigDB + ")", e2eCHURL, signals + "/internal/analytics/spinech/migrations"},
	} {
		fmt.Printf("==> migrating %s\n", m.label)
		if err := d.migrate(m.url, m.dir); err != nil {
			return sys.ExitCode(err)
		}
	}
	// Plugin-owned tables aren't in internal/db/migrations - a plugin ships its own, applied through the CLI, and without
	// this any suite touching a plugin's data fails on a missing relation
	fmt.Printf("==> migrating enabled plugins (%s)\n", e2eDB)
	if err := exec("exec", "-T", "-e", "DATABASE_URL="+e2eDBURL, "app", "go", "run", "./cmd/cli", "plugin", "migrate", "up"); err != nil {
		return sys.ExitCode(err)
	}

	// Stopped however the run ends - Ctrl-C included, which is why it's an AtExit and not a defer
	sys.AtExit(func() {
		_ = compose.Command(root, lib.Env(), "exec", "-T", "app", "sh", "-c", "pkill -f "+e2eBin).Run()
	})
	user := compose.UserFlags()
	fmt.Println("==> building the Signals process")
	if err := exec(append(append([]string{"exec", "-T"}, user...), "-w", signals, "app", "go", "build", "-o", e2eBin, "./cmd/signals")...); err != nil {
		return sys.ExitCode(err)
	}
	// The contract's secret - one value for core's tests and the Signals process, whatever the container env carries
	secret := os.Getenv("SIGNALS_SERVICE_SECRET")
	if secret == "" {
		secret = "e2e-service-secret"
	}
	fmt.Printf("==> starting it against %s\n", e2eSigDB)
	start := append(append([]string{"exec", "-T", "-d"}, user...),
		"-e", "SIGNALS_DB_URL="+e2eSigDS, "-e", "SIGNALS_CH_URL="+e2eCHURL, "-e", "SIGNALS_SERVICE_SECRET="+secret,
		"-e", "SIGNALS_GRPC_PORT="+e2eGRPC, "-e", "SIGNALS_PORT="+e2eHTTP,
		"app", "sh", "-c", "exec "+e2eBin+" > /tmp/skelet-e2e-signals.log 2>&1")
	if err := exec(start...); err != nil {
		return sys.ExitCode(err)
	}
	if !healthy(root, lib) {
		_ = exec("exec", "-T", "app", "cat", "/tmp/skelet-e2e-signals.log")
		fmt.Fprintln(os.Stderr, "the Signals process never answered /healthz")
		return 1
	}

	// plugins/... too: a plugin owning part of the money path carries its own scenarios against internal/e2ekit
	fmt.Println("==> running core's e2e suite")
	// One database per suite, not per package - the Signals process beside the scenarios reads the same one
	coreRun := append(append([]string{"exec", "-T"}, user...),
		"-e", "SKELET_TESTDB_SHARED=1", "-e", "DATABASE_URL="+e2eDBURL, "-e", "SIGNALS_DB_URL=",
		"-e", "E2E_SIGNALS_GRPC_ADDR=127.0.0.1:"+e2eGRPC, "-e", "SIGNALS_SERVICE_SECRET="+secret,
		"app", "go", "test", "./internal/e2e/...", "./plugins/...", "-count=1", "-timeout", "40m")
	if err := exec(append(coreRun, args...)...); err != nil {
		return sys.ExitCode(err)
	}
	fmt.Println("==> running the Signals module's e2e suite")
	sigRun := append(append([]string{"exec", "-T"}, user...), "-w", signals,
		"-e", "SKELET_TESTDB_SHARED=1", "-e", "SIGNALS_DB_URL="+e2eSigDS, "-e", "SIGNALS_CH_URL="+e2eCHURL,
		"app", "go", "test", "./internal/e2e/...", "-count=1", "-timeout", "40m")
	if err := exec(append(sigRun, args...)...); err != nil {
		return sys.ExitCode(err)
	}
	return 0
}

// How long healthy waits for the e2e Signals process - a minute
var healthTries, healthPause = 60, time.Second

func healthy(root string, lib compose.Lib) bool {
	for range healthTries {
		if compose.Command(root, lib.Env(), "exec", "-T", "app", "curl", "-sf", "http://localhost:"+e2eHTTP+"/healthz").Run() == nil {
			return true
		}
		time.Sleep(healthPause)
	}
	return false
}
