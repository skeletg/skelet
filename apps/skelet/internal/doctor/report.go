package doctor

import (
	"fmt"
	"io"
)

// report prints ok, warn and FAIL lines under sections and tallies them - only a FAIL makes doctor exit non-zero
type report struct {
	out            io.Writer
	ok, warn, fail int
}

func (r *report) section(title string, a ...any) { fmt.Fprintf(r.out, "==> "+title+"\n", a...) }

func (r *report) pass(format string, a ...any) {
	r.ok++
	fmt.Fprintf(r.out, "  ok    "+format+"\n", a...)
}

func (r *report) note(format string, a ...any) {
	r.warn++
	fmt.Fprintf(r.out, "  warn  "+format+"\n", a...)
}

func (r *report) bad(format string, a ...any) {
	r.fail++
	fmt.Fprintf(r.out, "  FAIL  "+format+"\n", a...)
}

func (r *report) end() int {
	fmt.Fprintf(r.out, "\n%d ok, %d warnings, %d failures\n", r.ok, r.warn, r.fail)
	if r.fail > 0 {
		return 1
	}
	return 0
}
