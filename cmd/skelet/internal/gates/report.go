// Package gates is skelet check: the gates that can fail a commit - version pins, repo structure, and the
// changelog's shape - run by the pre-commit hook and CI alike
package gates

import (
	"fmt"
	"io"
)

// report prints a gate's ok and FAIL lines and remembers whether any failed
type report struct {
	out    io.Writer
	failed bool
}

func (r *report) section(title string) { fmt.Fprintf(r.out, "==> %s\n", title) }

func (r *report) pass(format string, a ...any) { fmt.Fprintf(r.out, "  ok    "+format+"\n", a...) }

func (r *report) bad(format string, a ...any) {
	r.failed = true
	fmt.Fprintf(r.out, "  FAIL  "+format+"\n", a...)
}

func (r *report) code() int {
	if r.failed {
		return 1
	}
	return 0
}
