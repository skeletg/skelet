// Package tui draws setup's questions in the terminal - pickers, checkbox lists, a review screen, a spinner - on the
// standard library alone
package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInterrupted = errors.New("interrupted")
	ErrClosed      = errors.New("input closed")
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	accent = "\x1b[36m"
	green  = "\x1b[32m"
	red    = "\x1b[31m"
)

type Option struct {
	Label, Hint string
}

type UI struct {
	in    *bufio.Reader
	out   io.Writer
	raw   func() (func(), error)
	hush  func() (func(), error)
	width int
	drawn int
}

func New(in io.Reader, out io.Writer, raw func() (func(), error), width int) *UI {
	return &UI{in: bufio.NewReader(in), out: out, raw: raw, width: width}
}

func Terminal() *UI {
	u := New(os.Stdin, os.Stdout, func() (func(), error) { return Raw(os.Stdin.Fd()) }, Width(os.Stdout.Fd()))
	u.hush = func() (func(), error) { return Hush(os.Stdin.Fd()) }
	return u
}

func (u *UI) Section(title string, notes ...string) {
	fmt.Fprintf(u.out, "\n%s%s◆ %s%s\n", bold, accent, title, reset)
	if len(notes) > 0 {
		u.Note(strings.Join(notes, " "))
	}
}

func (u *UI) Note(text string) {
	for _, l := range wrap(text, max(u.width-4, 20)) {
		fmt.Fprintf(u.out, "%s  %s%s\n", dim, l, reset)
	}
}

func (u *UI) Quote(line string) {
	fmt.Fprintln(u.out, fit(dim+"  "+line+reset, u.width))
}

func (u *UI) Confirm(q string, def bool) (bool, error) {
	v := def
	err := u.session(func() error {
		for {
			u.draw(question(q)+"  "+pair(v), dim+"  y / n · ← → · enter"+reset)
			switch k := u.key(); {
			case k.is('y'):
				v = true
				return nil
			case k.is('n'):
				v = false
				return nil
			case k.kind == kLeft || k.kind == kRight || k.kind == kTab:
				v = !v
			case k.kind == kEnter:
				return nil
			case k.stop() != nil:
				return k.stop()
			}
		}
	})
	return v, u.finish(q, map[bool]string{true: "yes", false: "no"}[v], err)
}

func (u *UI) Select(q string, opts []Option, def int) (int, error) {
	at := max(0, min(def, len(opts)-1))
	err := u.session(func() error {
		for {
			lines := []string{question(q)}
			for i, o := range opts {
				lines = append(lines, row(i == at, "", o))
			}
			u.draw(append(lines, dim+"  ↑ ↓ · enter"+reset)...)
			switch k := u.key(); {
			case k.kind == kUp || k.is('k'):
				at = (at + len(opts) - 1) % len(opts)
			case k.kind == kDown || k.is('j') || k.kind == kTab:
				at = (at + 1) % len(opts)
			case k.kind == kRune && k.r >= '1' && int(k.r-'1') < len(opts):
				at = int(k.r - '1')
				return nil
			case k.kind == kEnter:
				return nil
			case k.stop() != nil:
				return k.stop()
			}
		}
	})
	return at, u.finish(q, opts[at].Label, err)
}

func (u *UI) Multi(q string, opts []Option, on []bool) ([]bool, error) {
	on = append([]bool(nil), on...)
	at := 0
	err := u.session(func() error {
		for {
			lines := []string{question(q)}
			for i, o := range opts {
				box := "[ ]"
				if on[i] {
					box = green + "[x]" + reset
				}
				lines = append(lines, row(i == at, box+" ", o))
			}
			u.draw(append(lines, dim+"  ↑ ↓ · space toggles · a all · enter"+reset)...)
			switch k := u.key(); {
			case k.kind == kUp || k.is('k'):
				at = (at + len(opts) - 1) % len(opts)
			case k.kind == kDown || k.is('j') || k.kind == kTab:
				at = (at + 1) % len(opts)
			case k.kind == kSpace || k.is('x'):
				on[at] = !on[at]
			case k.is('a'):
				all := !every(on)
				for i := range on {
					on[i] = all
				}
			case k.kind == kEnter:
				return nil
			case k.stop() != nil:
				return k.stop()
			}
		}
	})
	var picked []string
	for i, o := range opts {
		if on[i] {
			picked = append(picked, o.Label)
		}
	}
	return on, u.finish(q, orNone(picked), err)
}

func (u *UI) Input(q, def string, check func(string) string) (string, error) {
	var value []rune
	why := ""
	err := u.session(func() error {
		for {
			shown := string(value)
			if len(value) == 0 {
				shown = dim + def + reset
			}
			lines := []string{question(q) + "  " + accent + "›" + reset + " " + shown + "▏"}
			if why != "" {
				lines = append(lines, red+"  ✗ "+why+reset)
			}
			u.draw(append(lines, dim+"  enter takes it · ctrl-u clears"+reset)...)
			switch k := u.key(); {
			case k.kind == kRune || k.kind == kSpace:
				value, why = append(value, k.r), ""
			case k.kind == kBackspace && len(value) > 0:
				value = value[:len(value)-1]
			case k.kind == kClear:
				value = nil
			case k.kind == kEnter:
				v := strings.TrimSpace(string(value))
				if v == "" {
					v = def
				}
				if check != nil {
					if why = check(v); why != "" {
						continue
					}
				}
				value = []rune(v)
				return nil
			case k.stop() != nil:
				return k.stop()
			}
		}
	})
	return string(value), u.finish(q, string(value), err)
}

type Action int

const (
	Go Action = iota
	Edit
	Quit
)

func (u *UI) Review(title string, rows [][2]string) (Action, error) {
	wide := 0
	for _, r := range rows {
		wide = max(wide, len(r[0]))
	}
	act := Go
	body := []string{"", bold + accent + "◆ " + title + reset}
	for _, r := range rows {
		body = append(body, fmt.Sprintf("  %s%-*s%s  %s", dim, wide, r[0], reset, r[1]))
	}
	err := u.session(func() error {
		u.draw(append(body, "", "  "+bold+"enter"+reset+" go ahead · "+bold+"e"+reset+" change something · "+bold+"q"+reset+" quit, nothing written")...)
		for {
			switch k := u.key(); {
			case k.kind == kEnter:
				act = Go
				u.draw(body...)
				return nil
			case k.is('e'):
				act = Edit
				return nil
			case k.is('q') || k.kind == kEsc:
				act = Quit
				return nil
			case k.stop() != nil:
				return k.stop()
			}
		}
	})
	if err == nil && act != Go {
		u.wipe()
	}
	u.drawn = 0
	return act, err
}

var frames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func (u *UI) Spin(title string) func(ok bool) {
	quiet := func() {}
	if u.hush != nil {
		if undo, err := u.hush(); err == nil {
			quiet = undo
		}
	}
	done, gone := make(chan bool), make(chan struct{})
	go func() {
		defer close(gone)
		tick := time.NewTicker(90 * time.Millisecond)
		defer tick.Stop()
		for i := 0; ; i++ {
			fmt.Fprintf(u.out, "\r\x1b[2K%s%c%s %s…", accent, frames[i%len(frames)], reset, title)
			select {
			case ok := <-done:
				mark := green + "✓" + reset
				if !ok {
					mark = red + "✗" + reset
				}
				fmt.Fprintf(u.out, "\r\x1b[2K%s %s\n", mark, title)
				return
			case <-tick.C:
			}
		}
	}()
	return func(ok bool) {
		done <- ok
		<-gone
		quiet()
	}
}

func (u *UI) session(fn func() error) error {
	restore, err := u.raw()
	if err != nil {
		return err
	}
	fmt.Fprint(u.out, "\x1b[?25l")
	defer func() {
		fmt.Fprint(u.out, "\x1b[?25h")
		restore()
	}()
	return fn()
}

func (u *UI) finish(q, answer string, err error) error {
	if err != nil {
		u.drawn = 0
		fmt.Fprintln(u.out)
		return err
	}
	u.wipe()
	fmt.Fprintln(u.out, fit(fmt.Sprintf("  %s✓%s %s %s·%s %s%s%s", green, reset, q, dim, reset, accent, answer, reset), u.width))
	return nil
}

func (u *UI) draw(lines ...string) {
	var b strings.Builder
	if u.drawn > 0 {
		fmt.Fprintf(&b, "\x1b[%dA\r\x1b[J", u.drawn)
	}
	for _, l := range lines {
		b.WriteString(fit(l, u.width) + "\r\n")
	}
	u.drawn = len(lines)
	fmt.Fprint(u.out, b.String())
}

func (u *UI) wipe() {
	if u.drawn > 0 {
		fmt.Fprintf(u.out, "\x1b[%dA\r\x1b[J", u.drawn)
	}
	u.drawn = 0
}

func question(q string) string { return accent + "?" + reset + " " + bold + q + reset }

func pair(yes bool) string {
	if yes {
		return accent + bold + "‹ yes ›" + reset + dim + "   no" + reset
	}
	return dim + "  yes   " + reset + accent + bold + "‹ no ›" + reset
}

func row(at bool, mark string, o Option) string {
	cursor := "  "
	label := o.Label
	if at {
		cursor, label = accent+"❯ "+reset, bold+o.Label+reset
	}
	line := "  " + cursor + mark + label
	if o.Hint != "" {
		line += "  " + dim + o.Hint + reset
	}
	return line
}

func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for w := range strings.FieldsSeq(text) {
		switch {
		case line == "":
			line = w
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(w) > width:
			lines, line = append(lines, line), w
		default:
			line += " " + w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func every(on []bool) bool {
	for _, v := range on {
		if !v {
			return false
		}
	}
	return true
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
}

// fit cuts a line to the terminal's width by what shows, so a redraw never counts a wrapped line as one
func fit(line string, width int) string {
	limit := max(width-1, 10)
	if utf8.RuneCountInString(stripped(line)) <= limit {
		return line
	}
	var b strings.Builder
	shown := 0
	for i := 0; i < len(line) && shown < limit-1; {
		if line[i] == '\x1b' {
			n := escLen(line[i:])
			b.WriteString(line[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		b.WriteRune(r)
		shown++
		i += size
	}
	return b.String() + "…" + reset
}

func stripped(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			i += escLen(s[i:])
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func escLen(s string) int {
	if end := strings.IndexAny(s, "mAJKH"); end >= 0 {
		return end + 1
	}
	return len(s)
}
