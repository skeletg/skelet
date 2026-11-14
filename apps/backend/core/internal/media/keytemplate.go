package media

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrKeyTemplate = errors.New("invalid file path template")

const (
	keyTemplateMax = 200
	keyPartMax     = 100 // a slot's share of a key, which storage caps at 1024 bytes in all
	slotExt        = "{ext}"
	keyFallback    = "file"
)

var (
	keySlots     = []string{"id", "short_id", "name", "yyyy", "mm", "dd"}
	keyUnsafe    = regexp.MustCompile(`[^a-z0-9._-]+`)
	keyDotRun    = regexp.MustCompile(`\.{2,}`)
	keyFileText  = regexp.MustCompile(`^[a-zA-Z0-9._-]*$`)
	keyTreeText  = regexp.MustCompile(`^[a-zA-Z0-9._/-]*$`)
	keySlotShape = regexp.MustCompile(`\{[^{}]*\}`)
)

// A storage key written as text between {slots}, the way "{id}-{name}{ext}" is
type KeyTemplate struct {
	parts []keyPart
}

type keyPart struct {
	text string
	slot bool
}

type KeyValues struct {
	ID   uuid.UUID
	Name string
	Ext  string
	At   time.Time
	More map[string]string // the slots only one kind of file has, like a product's sku
}

// folders lets the template hold a path rather than one file name, more names the slots beyond the shared ones
func ParseKeyTemplate(s string, folders bool, more ...string) (KeyTemplate, error) {
	s = strings.TrimSpace(s)
	whole, ended := strings.CutSuffix(s, slotExt)
	switch {
	case s == "":
		return KeyTemplate{}, bad("it's empty")
	case len(s) > keyTemplateMax:
		return KeyTemplate{}, bad("it's longer than %d characters", keyTemplateMax)
	case !ended:
		return KeyTemplate{}, bad("it has to end with %s", slotExt)
	}
	slots := append(slices.Clone(keySlots), more...)
	text, allowed := keyFileText, "letters, digits, dots, dashes and underscores"
	if folders {
		text, allowed = keyTreeText, allowed+", with / between folders"
	}
	var t KeyTemplate
	unique := false
	for body := whole; body != ""; {
		lit, rest, opened := strings.Cut(body, "{")
		if strings.Contains(lit, "}") {
			return KeyTemplate{}, bad("a } closes nothing")
		}
		if !text.MatchString(lit) {
			return KeyTemplate{}, bad("%q can't be part of a file's address - only %s", lit, allowed)
		}
		if lit != "" {
			t.parts = append(t.parts, keyPart{text: lit})
		}
		if !opened {
			break
		}
		name, after, closed := strings.Cut(rest, "}")
		if !closed || strings.Contains(name, "{") {
			return KeyTemplate{}, bad("a { is never closed")
		}
		if !slices.Contains(slots, name) {
			return KeyTemplate{}, bad("{%s} isn't a placeholder here - there's {%s} and %s", name, strings.Join(slots, "}, {"), slotExt)
		}
		unique = unique || name == "id" || name == "short_id"
		t.parts = append(t.parts, keyPart{text: name, slot: true})
		body = after
	}
	if !unique {
		// resized copies are cached under the original's key, so a key that came back would show the old picture
		return KeyTemplate{}, bad("it needs {id} or {short_id}, so an address is never used twice")
	}
	for seg := range strings.SplitSeq(keySlotShape.ReplaceAllString(whole, "x"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return KeyTemplate{}, bad("every folder in it needs a name, and not . or ..")
		}
	}
	// the image resizer refuses any key holding "..", wherever it sits
	if strings.Contains(whole, "..") || strings.HasSuffix(whole, ".") {
		return KeyTemplate{}, bad("two dots can't sit side by side, and %s brings its own", slotExt)
	}
	return t, nil
}

// Reads as the reason alone, since it's shown to the admin right after the field's label
type keyTemplateError string

func (e keyTemplateError) Error() string        { return string(e) }
func (e keyTemplateError) Is(target error) bool { return target == ErrKeyTemplate }

func bad(format string, args ...any) error {
	return keyTemplateError(fmt.Sprintf(format, args...))
}

func (t KeyTemplate) Key(v KeyValues) string {
	var b strings.Builder
	for _, p := range t.parts {
		if p.slot {
			b.WriteString(v.slot(p.text))
		} else {
			b.WriteString(p.text)
		}
	}
	return b.String() + v.Ext
}

func (v KeyValues) slot(name string) string {
	at := v.At.UTC()
	switch name {
	case "id":
		return v.ID.String()
	case "short_id":
		id := v.ID.String()
		return id[len(id)-8:] // the tail is random, the head of a v7 id is the clock
	case "name":
		return KeyPart(strings.TrimSuffix(v.Name, path.Ext(v.Name)))
	case "yyyy":
		return at.Format("2006")
	case "mm":
		return at.Format("01")
	case "dd":
		return at.Format("02")
	}
	return KeyPart(v.More[name])
}

func KeyPart(s string) string {
	s = keyDotRun.ReplaceAllString(keyUnsafe.ReplaceAllString(strings.ToLower(s), "-"), ".")
	s = strings.Trim(s[:min(len(s), keyPartMax)], "-.")
	if s == "" {
		return keyFallback
	}
	return s
}
