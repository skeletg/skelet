package testrun

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// Disposable test databases. testdb.Require refuses any database without the skelet.e2e marker, since database-backed
// tests write real rows and the dev databases never carry it - so a suite brings its own: created here, marked here,
// and the dev ones unmarked again so a stray go test against dev refuses
type dbs struct {
	root string
	lib  compose.Lib
}

func (d dbs) compose(args ...string) *exec.Cmd { return compose.Command(d.root, d.lib.Env(), args...) }

// sql runs psql as the instance superuser against the always-present maintenance database, so it can create and drop
// the target - ON_ERROR_STOP so a failure isn't swallowed
func (d dbs) sql(service, query string) (string, error) {
	cmd := d.compose("exec", "-T", service, "psql", "-U", "skelet", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-tAc", query)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func (d dbs) exists(service, name string) (bool, error) {
	out, err := d.sql(service, "SELECT 1 FROM pg_database WHERE datname='"+name+"'")
	return out == "1", err
}

// postgres makes the database if it's missing - fresh drops it first - and marks it disposable before anything
// connects, since the marker takes effect on new connections
func (d dbs) postgres(service, name string, fresh bool) error {
	there, err := d.exists(service, name)
	if err != nil {
		return err
	}
	if fresh && there {
		fmt.Printf("==> dropping %s\n", name)
		if _, err := d.sql(service, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			return err
		}
		there = false
	}
	if !there {
		fmt.Printf("==> creating %s\n", name)
		if _, err := d.sql(service, "CREATE DATABASE "+name); err != nil {
			return err
		}
	}
	_, err = d.sql(service, "ALTER DATABASE "+name+" SET skelet.e2e = 'allowed'")
	return err
}

// dropClones sweeps the per-package copies testdb makes of a template - throwaways of a throwaway - in one psql fed on
// stdin, so each DROP runs on its own as it must. An exec per clone was most of setup
func (d dbs) dropClones(service, template string) error {
	names, err := d.sql(service, "SELECT datname FROM pg_database WHERE datname LIKE '"+template+`\_\_pkg%'`)
	if err != nil || names == "" {
		return err
	}
	var drops strings.Builder
	for n := range strings.FieldsSeq(names) {
		fmt.Fprintf(&drops, "DROP DATABASE \"%s\" WITH (FORCE);\n", n)
	}
	cmd := d.compose("exec", "-T", service, "psql", "-U", "skelet", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-q")
	cmd.Stdin, cmd.Stderr = strings.NewReader(drops.String()), os.Stderr
	return cmd.Run()
}

func (d dbs) clickhouse(query string, multi bool) (string, error) {
	args := []string{"exec", "-T", "clickhouse", "clickhouse-client", "--user=skelet", "--password=skelet"}
	if multi {
		args = append(args, "--multiquery")
	}
	cmd := d.compose(append(args, "--query", query)...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// clickhouseDB is postgres for the spine: the marker lives in the database comment
func (d dbs) clickhouseDB(name string, fresh bool) error {
	if fresh {
		if _, err := d.clickhouse("DROP DATABASE IF EXISTS "+name, false); err != nil {
			return err
		}
	}
	if _, err := d.clickhouse("CREATE DATABASE IF NOT EXISTS "+name, false); err != nil {
		return err
	}
	_, err := d.clickhouse("ALTER DATABASE "+name+" MODIFY COMMENT 'skelet.e2e=allowed'", false)
	return err
}

func (d dbs) dropClickhouseClones(template string) error {
	names, err := d.clickhouse("SELECT name FROM system.databases WHERE startsWith(name, '"+template+"__pkg')", false)
	if err != nil || names == "" {
		return err
	}
	var drops strings.Builder
	for n := range strings.FieldsSeq(names) {
		fmt.Fprintf(&drops, "DROP DATABASE IF EXISTS `%s`;", n)
	}
	_, err = d.clickhouse(drops.String(), true)
	return err
}

// unmarkDev keeps the dev databases unmarked, so a stray go test against dev refuses instead of writing test rows
func (d dbs) unmarkDev() {
	_, _ = d.sql("db", "ALTER DATABASE skelet RESET skelet.e2e")
	_, _ = d.sql("signals-db", "ALTER DATABASE skelet_signals RESET skelet.e2e")
}

// migrate runs dbmate in the app container against url
func (d dbs) migrate(url, dir string) error {
	cmd := d.compose("exec", "-T", "app", "dbmate", "--no-dump-schema", "--url", url, "--migrations-dir", dir, "up")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return sys.Run(cmd)
}

var errLocked = errors.New("locked")

// lock holds the test databases for one run per suite and checkout - a second run's DROP ... WITH (FORCE) would pull
// them from under the first. mkdir is the atomic step; a lock whose holder died is taken over, through a second
// directory so two runs can't both take it. It's released by the AtExit stack, which Ctrl-C runs too
func lock(suite, root string) error {
	dir := lockPath(suite, root)
	if err := os.Mkdir(dir, 0o700); err != nil {
		holder := readPid(dir)
		if holder > 0 && syscall.Kill(holder, 0) == nil {
			fmt.Fprintf(os.Stderr, "another %s run (pid %d) is using the test databases - wait for it to finish\n", suite, holder)
			return errLocked
		}
		takeover := dir + ".takeover"
		if os.Mkdir(takeover, 0o700) != nil {
			fmt.Fprintf(os.Stderr, "another %s run just started - wait for it to finish\n", suite)
			return errLocked
		}
		if readPid(dir) != holder {
			_ = os.Remove(takeover)
			fmt.Fprintf(os.Stderr, "another %s run just started - wait for it to finish\n", suite)
			return errLocked
		}
		_ = os.RemoveAll(dir)
		err := os.Mkdir(dir, 0o700)
		_ = os.Remove(takeover)
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return err
	}
	sys.AtExit(func() { _ = os.RemoveAll(dir) })
	return nil
}

func lockPath(suite, root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(os.TempDir(), "skelet-"+suite+"-"+hex.EncodeToString(sum[:])[:12]+".lock")
}

func readPid(dir string) int {
	raw, _ := os.ReadFile(filepath.Join(dir, "pid"))
	n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return n
}
