package testrun

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The streams are real go test -json runs - passes, skips with and without reasons, a subtest, a failure logging
// escaped text, a TestMain panic and a build error - and the goldens are what the bash summarizer printed for them
func TestSummarizeMatchesTheBashTally(t *testing.T) {
	for _, name := range []string{"ok", "fail", "panics", "broken", "all"} {
		in, err := os.Open("testdata/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		want, _ := os.ReadFile("testdata/" + name + ".golden")
		var out bytes.Buffer
		summarize(name, in, &out)
		_ = in.Close()
		if out.String() != string(want) {
			t.Errorf("%s:\n%s\nwant:\n%s", name, out.String(), want)
		}
	}
}

// A line longer than bufio.Scanner's 64KB default - a test that logs a big payload - must not end the tally early
func TestSummarizeTakesLongLines(t *testing.T) {
	long := `{"Action":"output","Package":"p","Test":"T","Output":"` + strings.Repeat("x", 200_000) + `\n"}` + "\n"
	stream := long + `{"Action":"pass","Package":"p","Test":"T"}` + "\n" + `{"Action":"skip","Package":"p","Test":"S"}` + "\n"
	var out bytes.Buffer
	summarize("p", strings.NewReader(stream), &out)
	if !strings.Contains(out.String(), "==> p: 1 passed, 0 failed, 1 skipped") {
		t.Errorf("tally: %q", out.String())
	}
}

func TestSummarizeIgnoresNonJSON(t *testing.T) {
	var out bytes.Buffer
	summarize("p", strings.NewReader("go: downloading x\n"+`{"Action":"pass","Package":"p","Test":"T"}`+"\n"), &out)
	if out.String() != "==> p: 1 passed, 0 failed, 0 skipped\n" {
		t.Errorf("%q", out.String())
	}
}
