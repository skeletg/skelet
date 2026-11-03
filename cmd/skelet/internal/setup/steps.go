package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
	"github.com/skeletg/skelet/cmd/skelet/internal/tui"
)

// stepper runs a stage behind a spinner when the questions are drawn, what it prints going to the log
type stepper struct {
	ui      *tui.UI
	logPath string
	log     *os.File
	mu      sync.Mutex
	stop    func(bool)
}

func newStepper(root string, ui *tui.UI) (*stepper, error) {
	s := &stepper{ui: ui}
	if ui == nil {
		return s, nil
	}
	s.logPath = filepath.Join(root, ".skelet", "setup.log")
	if err := os.MkdirAll(filepath.Dir(s.logPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(s.logPath)
	if err != nil {
		return nil, err
	}
	s.log = f
	sys.AtExit(func() { s.end(false) })
	return s, nil
}

func (s *stepper) run(title string, fn func() int) int {
	if s.ui == nil {
		return fn()
	}
	fmt.Fprintf(s.log, "\n==> %s\n", title)
	from, _ := s.log.Seek(0, io.SeekCurrent)
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = s.log, s.log
	s.mu.Lock()
	s.stop = s.ui.Spin(title)
	s.mu.Unlock()
	code := fn()
	os.Stdout, os.Stderr = stdout, stderr
	s.end(code == 0)
	if code != 0 {
		s.tail(from, 20)
	}
	return code
}

func (s *stepper) end(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		s.stop(ok)
		s.stop = nil
	}
}

func (s *stepper) tail(from int64, n int) {
	raw, err := os.ReadFile(s.logPath)
	if err != nil || from > int64(len(raw)) {
		return
	}
	lines := strings.Split(strings.TrimRight(string(raw[from:]), "\n"), "\n")
	for _, l := range lines[max(0, len(lines)-n):] {
		s.ui.Quote(l)
	}
	s.ui.Quote("the whole log: " + s.logPath)
}

func (s *stepper) close() {
	if s.log != nil {
		_ = s.log.Close()
	}
}

func visual(f flags) bool {
	for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if !sys.IsTerminal(file.Fd()) {
			return false
		}
	}
	return !f.yes && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}
