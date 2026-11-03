// Package db holds the verbs for the dev databases: psql, migrations, and dumping and restoring the commerce one
package db

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const (
	ShellAbout   = `psql on the dev commerce database - skelet psql -c 'select 1', or no arguments for a prompt.`
	MigrateAbout = `dbmate on a dev database: up | rollback | status | new <name>. With --signals, anywhere in the
arguments, the Signals database - run in the signals container, the one process holding its address.
The app and the signals process migrate on boot, so this is for rolling back and for new migrations.`
	ExportAbout = `Dumps the dev database to backups/skelet-<time>.dump, or to the file you name - a .sql name writes
plain SQL. It writes to <file>.partial and renames it when the dump finishes, so a failed dump never
looks like a finished one.`
	ImportAbout = `Restores a dump over the dev database, which it OVERWRITES: skelet db import [-y] <file.dump|file.sql>.
It asks first unless you pass -y, and refuses without a terminal to ask on.`
)

func Shell(root string, args []string) int {
	return call(root, append([]string{"exec", "db", "psql", "-U", "skelet", "-d", "skelet"}, args...)...).Replace()
}

func Migrate(root string, args []string) int {
	return call(root, append([]string{"exec", "app", "sh", "-c",
		`dbmate --no-dump-schema --migrations-dir internal/db/migrations "$@"`, "--"}, args...)...).Replace()
}

func MigrateSignals(root string, args []string) int {
	return call(root, append([]string{"exec", "signals", "sh", "-c",
		`test -n "$SIGNALS_DB_URL" || { echo "SIGNALS_DB_URL unset - required, check docker-compose.yml" >&2; exit 1; }
dbmate --no-dump-schema --env SIGNALS_DB_URL --migrations-dir /var/www/backend/signals/internal/db/migrations "$@"`, "--"}, args...)...).Replace()
}

func call(root string, args ...string) sys.Call {
	return compose.Call(root, compose.Load(root).Env(), args...)
}

func Export(root string, args []string) int {
	out := ""
	if len(args) > 0 {
		out = fromCaller(args[0])
	} else {
		dir := filepath.Join(root, "backups")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail(err)
		}
		out = filepath.Join(dir, "skelet-"+time.Now().Format("20060102-150405")+".dump")
	}
	dump := []string{"exec", "-T", "db", "pg_dump", "-U", "skelet", "-d", "skelet"}
	if !strings.HasSuffix(out, ".sql") {
		dump = append(dump, "-Fc")
	}
	partial := out + ".partial"
	f, err := os.Create(partial)
	if err != nil {
		return fail(err)
	}
	cmd := call(root, dump...).Cmd()
	cmd.Stdout, cmd.Stderr = f, os.Stderr
	err = sys.Run(cmd)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(partial)
		fmt.Fprintln(os.Stderr, "skelet db export: the dump failed, nothing was written")
		return max(sys.ExitCode(err), 1)
	}
	if err := os.Rename(partial, out); err != nil {
		return fail(err)
	}
	fi, _ := os.Stat(out)
	fmt.Printf("exported → %s (%s)\n", out, size(fi.Size()))
	return 0
}

func Import(root string, args []string) int {
	yes := len(args) > 0 && (args[0] == "-y" || args[0] == "--yes")
	if yes {
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(os.Stderr, "usage: skelet db import [-y] <file.dump|file.sql>")
		return 1
	}
	file := fromCaller(args[0])
	if fi, err := os.Stat(file); err != nil || !fi.Mode().IsRegular() {
		fmt.Fprintf(os.Stderr, "no such file: %s\n", file)
		return 1
	}
	if !yes {
		// Opened, not just looked for - /dev/tty exists without a terminal to read from, in CI and cron
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "refusing to overwrite without a terminal - re-run with -y to confirm")
			return 1
		}
		fmt.Fprintf(tty, "This OVERWRITES the dev database (skelet) with %s. Continue? [y/N] ", file)
		answer, _ := bufio.NewReader(tty).ReadString('\n')
		_ = tty.Close()
		switch strings.TrimSpace(answer) {
		case "y", "Y", "yes", "YES":
		default:
			fmt.Println("aborted")
			return 1
		}
	}
	restore := []string{"exec", "-T", "db", "pg_restore", "-U", "skelet", "-d", "skelet", "--clean", "--if-exists", "--no-owner"}
	if strings.HasSuffix(file, ".sql") {
		restore = []string{"exec", "-T", "db", "psql", "-U", "skelet", "-d", "skelet"}
	}
	in, err := os.Open(file)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = in.Close() }()
	cmd := call(root, restore...).Cmd()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, os.Stdout, os.Stderr
	if err := sys.Run(cmd); err != nil {
		return sys.ExitCode(err)
	}
	fmt.Printf("imported ← %s\n", file)
	return 0
}

// fromCaller resolves a path the way the person typing it meant: against the directory they ran skelet from
func fromCaller(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	return filepath.Join(cwd, p)
}

// size is du -h's shape: one decimal below ten, whole numbers above
func size(n int64) string {
	units := []string{"", "K", "M", "G", "T"}
	v, i := float64(n), 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i > 0 && v < 10 {
		return fmt.Sprintf("%.1f%s", v, units[i])
	}
	return fmt.Sprintf("%.0f%s", v, units[i])
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "skelet db:", err)
	return 1
}
