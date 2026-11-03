// Package testrun is skelet test and its two siblings: the Go and frontend suites against disposable databases, the
// money-path e2e suite, and a storefront's browser tests
package testrun

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

// The go test -json event fields the tally reads
type event struct {
	Action      string
	Package     string
	Test        string
	Output      string
	FailedBuild string
}

var (
	verdict    = regexp.MustCompile(`^(PASS|FAIL)\n$`)
	skipPrefix = regexp.MustCompile(`^\s*\S+\.go:[0-9]+: `)
)

// summarize turns go test -json into the usual go test output plus a tally - a skip is invisible in the plain output,
// and a skip is exactly what a missing database looks like. Passing tests' logs are dropped as go test drops them
// without -v; a failing test prints everything it logged
func summarize(label string, in io.Reader, out io.Writer) {
	var (
		passed, failed, skipped int
		buf, last               = map[string]string{}, map[string]string{}
		testFailed              = map[string]bool{}
		broken                  []string
		why                     = map[string]string{}
		reasons                 []string
		counts                  = map[string]int{}
	)
	r := bufio.NewReaderSize(in, 1<<20)
	for {
		line, err := r.ReadString('\n')
		var e event
		if line != "" && json.Unmarshal([]byte(line), &e) == nil {
			key := e.Package + "\x00" + e.Test
			switch {
			// The compiler's errors - they carry ImportPath, not Package
			case e.Action == "build-output":
				fmt.Fprint(out, e.Output)
			case e.Action == "output" && e.Test == "":
				// ok / FAIL / ? lines, build errors, panics - but not the binary's bare verdict, which the ok/FAIL line
				// after it carries
				if !verdict.MatchString(e.Output) {
					fmt.Fprint(out, e.Output)
				}
			case e.Action == "output":
				if !strings.HasPrefix(e.Output, "=== ") {
					buf[key] += e.Output
					if !strings.HasPrefix(e.Output, "--- ") && strings.TrimSpace(e.Output) != "" {
						last[key] = e.Output
					}
				}
			// A package-level fail is the verdict on tests already counted - unless none of them failed, which is a build
			// failure or a binary that died outside any test, and would otherwise leave the tally at 0 failed
			case e.Test == "":
				if e.Action == "fail" && !testFailed[e.Package] && !slices.Contains(broken, e.Package) {
					broken = append(broken, e.Package)
					why[e.Package] = "failed outside any test"
					if e.FailedBuild != "" {
						why[e.Package] = "build failed"
					}
				}
			case e.Action == "pass":
				passed++
				delete(buf, key)
				delete(last, key)
			case e.Action == "fail":
				failed++
				testFailed[e.Package] = true
				fmt.Fprint(out, buf[key])
				delete(buf, key)
				delete(last, key)
			case e.Action == "skip":
				skipped++
				reason := strings.TrimSuffix(skipPrefix.ReplaceAllString(last[key], ""), "\n")
				if reason == "" {
					reason = "(no reason given)"
				}
				if counts[reason] == 0 {
					reasons = append(reasons, reason)
				}
				counts[reason]++
				delete(buf, key)
				delete(last, key)
			}
		}
		if err != nil {
			break
		}
	}
	fmt.Fprintf(out, "==> %s: %d passed, %d failed, %d skipped", label, passed, failed, skipped)
	if n := len(broken); n > 0 {
		plural := "s"
		if n == 1 {
			plural = ""
		}
		fmt.Fprintf(out, ", %d package%s broken", n, plural)
	}
	fmt.Fprintln(out)
	for _, p := range broken {
		fmt.Fprintf(out, "    broken: %s (%s)\n", p, why[p])
	}
	// The commonest reason first - that's the one worth chasing - and ties in the order they turned up
	slices.SortStableFunc(reasons, func(a, b string) int { return counts[b] - counts[a] })
	for _, reason := range reasons {
		fmt.Fprintf(out, "    %4d skipped: %s\n", counts[reason], reason)
	}
}
