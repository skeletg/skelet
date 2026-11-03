package tui

import "unicode/utf8"

type kind int

const (
	kRune kind = iota
	kEnter
	kUp
	kDown
	kLeft
	kRight
	kSpace
	kBackspace
	kTab
	kEsc
	kClear
	kInterrupt
	kEOF
	kOther
)

type key struct {
	kind kind
	r    rune
}

func (k key) is(r rune) bool { return k.kind == kRune && (k.r == r || k.r == r-32) }

func (k key) stop() error {
	switch k.kind {
	case kInterrupt:
		return ErrInterrupted
	case kEOF:
		return ErrClosed
	}
	return nil
}

func (u *UI) key() key {
	b, err := u.in.ReadByte()
	if err != nil {
		return key{kind: kEOF}
	}
	switch b {
	case 3:
		return key{kind: kInterrupt}
	case '\r', '\n':
		return key{kind: kEnter}
	case ' ':
		return key{kind: kSpace, r: ' '}
	case 127, 8:
		return key{kind: kBackspace}
	case '\t':
		return key{kind: kTab}
	case 21:
		return key{kind: kClear}
	case 0x1b:
		return u.escape()
	}
	if b < 0x20 {
		return key{kind: kOther}
	}
	if b < utf8.RuneSelf {
		return key{kind: kRune, r: rune(b)}
	}
	_ = u.in.UnreadByte()
	r, _, err := u.in.ReadRune()
	if err != nil {
		return key{kind: kEOF}
	}
	return key{kind: kRune, r: r}
}

// escape tells an arrow's sequence from Esc on its own - a sequence arrives in one read, a lone Esc with nothing after
func (u *UI) escape() key {
	if u.in.Buffered() == 0 {
		return key{kind: kEsc}
	}
	if next, err := u.in.ReadByte(); err != nil || (next != '[' && next != 'O') {
		return key{kind: kOther}
	}
	for range 8 {
		c, err := u.in.ReadByte()
		if err != nil {
			return key{kind: kEOF}
		}
		if c >= 0x40 && c <= 0x7e {
			switch c {
			case 'A':
				return key{kind: kUp}
			case 'B':
				return key{kind: kDown}
			case 'C':
				return key{kind: kRight}
			case 'D':
				return key{kind: kLeft}
			}
			return key{kind: kOther}
		}
	}
	return key{kind: kOther}
}
