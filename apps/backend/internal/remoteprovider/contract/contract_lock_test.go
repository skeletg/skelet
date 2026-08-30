package contract

// The drift gate for the wire contract: contract.lock records Version plus a sha256 per
// generated schema/example file, and this test refuses any schema change that lands without
// a Version bump. Unlike the SDK's apidiff gate this one can't classify breaking vs additive
// mechanically - a changed .schema.json may be a removed field or a new optional one - so it
// enforces that A bump happened and leaves picking minor-vs-patch to the author, with the
// policy spelled out in the failure message
//
// Regenerate after a deliberate change: go test ./internal/remoteprovider/contract -run TestContractLock -update-provider-contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var updateLock = flag.Bool("update-provider-contract", false, "rewrite contract.lock with the current schema hashes")

const lockFile = "contract.lock" // deliberately not .json so the embed glob never picks it up

type lock struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func TestContractLock(t *testing.T) {
	cur := lock{Version: Version, Files: hashFiles(t)}
	if *updateLock {
		writeLock(t, cur)
		return
	}

	raw, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("read %s: %v - bootstrap with: go test ./internal/remoteprovider/contract -run TestContractLock -update-provider-contract", lockFile, err)
	}
	var old lock
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatalf("parse %s: %v", lockFile, err)
	}
	if old.Version != Version {
		t.Fatalf("contract.Version is %s but %s records %s - version and lock move together", Version, lockFile, old.Version)
	}

	changed, added, removed := diffHashes(old.Files, cur.Files)
	if len(changed)+len(added)+len(removed) == 0 {
		return
	}
	var b strings.Builder
	report := func(label string, names []string) {
		if len(names) > 0 {
			fmt.Fprintf(&b, "%s:\n  %s\n", label, strings.Join(names, "\n  "))
		}
	}
	report("changed", changed)
	report("added", added)
	report("removed", removed)
	t.Fatalf(`wire contract drifted from %s:
%sproviders consume this over HTTP and can't recompile alongside core, so every change ships under a new contract.Version:
  breaking (removed/renamed field, changed type, new required expectation, removed kind) - bump the minor
  additive (new kind, new optional field) - bump the patch
then regenerate: go test ./internal/remoteprovider/contract -run TestContractLock -update-provider-contract`, lockFile, b.String())
}

// Refuses a rewrite under the recorded version, so the lock can't be regenerated without the bump
// the checker above just demanded
func writeLock(t *testing.T, cur lock) {
	t.Helper()
	if raw, err := os.ReadFile(lockFile); err == nil {
		var old lock
		if err := json.Unmarshal(raw, &old); err != nil {
			t.Fatalf("parse %s: %v", lockFile, err)
		}
		changed, added, removed := diffHashes(old.Files, cur.Files)
		drifted := len(changed)+len(added)+len(removed) > 0
		if drifted && !versionAdvanced(old.Version, Version) {
			t.Fatalf("refusing to record contract changes under version %s (lock has %s) - bump contract.Version first", Version, old.Version)
		}
	}
	out, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockFile, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s: v%s, %d files", lockFile, Version, len(cur.Files))
}

// Compares dotted-numeric versions the same way the SDK's semver does, without importing it -
// contract is a leaf package and stays one - an unparseable component compares as 0
func versionAdvanced(old, cur string) bool {
	o, c := strings.Split(old, "."), strings.Split(cur, ".")
	for i := 0; i < len(o) && i < len(c); i++ {
		ov, _ := strconv.Atoi(o[i])
		cv, _ := strconv.Atoi(c[i])
		if cv != ov {
			return cv > ov
		}
	}
	return len(c) > len(o)
}

func hashFiles(t *testing.T) map[string]string {
	t.Helper()
	names, err := fs.Glob(files, "*.json")
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		b, err := files.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		out[n] = hex.EncodeToString(sum[:])
	}
	return out
}

func diffHashes(old, cur map[string]string) (changed, added, removed []string) {
	for n, h := range cur {
		oldH, ok := old[n]
		switch {
		case !ok:
			added = append(added, n)
		case oldH != h:
			changed = append(changed, n)
		}
	}
	for n := range old {
		if _, ok := cur[n]; !ok {
			removed = append(removed, n)
		}
	}
	sort.Strings(changed)
	sort.Strings(added)
	sort.Strings(removed)
	return changed, added, removed
}
