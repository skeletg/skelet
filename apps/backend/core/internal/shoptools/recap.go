package shoptools

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// A recap rides back with its reply in the conversation, so it's kept short
const maxRecapChars = 600

// The recap's lines, written for the model to read back - ReadRecap below is the only reader of them besides it
const (
	recapPanel = "Explained in a panel: "
	recapCards = "Shown as cards: "
	recapLink  = "Button "
	recapMoved = "Moved the shopper's list to "
	recapCart  = "Offered the Review cart button"
)

// recap is what the reply showed beyond its words - the conversation the browser sends back carries only words, so
// without it "compare it with this one" meant a product the model knew by name alone, and it went looking by name
func (c *conversation) recap() string {
	lines := slices.Clone(c.returns) // first, so the clip never takes the key a confirmation looks for
	if c.detail != nil {
		lines = append(lines, recapPanel+c.named(c.detail.ProductID))
	}
	if len(c.shown) > 0 {
		cards := make([]string, len(c.shown))
		for i, id := range c.shown {
			cards[i] = c.named(id)
		}
		lines = append(lines, cardsLine(cards))
	}
	for _, l := range c.links {
		lines = append(lines, fmt.Sprintf("%s%q to %s", recapLink, l.Label, l.Path))
	}
	if c.navigate != nil {
		lines = append(lines, recapMoved+c.navigate.Path)
	}
	if c.checkout {
		lines = append(lines, recapCart)
	}
	return clip(strings.Join(lines, ". "), maxRecapChars)
}

func cardsLine(named []string) string { return recapCards + strings.Join(named, "; ") }

func nameWithID(name string, id uuid.UUID) string {
	if name != "" {
		return fmt.Sprintf("%s (id %s)", name, id)
	}
	return "id " + id.String()
}

func (c *conversation) named(id uuid.UUID) string { return nameWithID(c.names[id], id) }

func (c *conversation) remember(id uuid.UUID, name string) {
	if c.names == nil {
		c.names = map[uuid.UUID]string{}
	}
	if name != "" {
		c.names[id] = name
	}
}

// OverviewRecap is the recap of the products an overview names - all of them when it names none - so a chat it's
// carried into knows them as the cards it showed
func OverviewRecap(o Overview) string {
	var named []string
	seen := map[int]bool{}
	for _, m := range overviewRef.FindAllStringSubmatch(o.Text, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= len(o.Products) && !seen[n] {
			seen[n] = true
			named = append(named, nameWithID(o.Products[n-1].Name, o.Products[n-1].ID))
		}
	}
	if len(named) == 0 {
		for _, p := range o.Products {
			named = append(named, nameWithID(p.Name, p.ID))
		}
	}
	if len(named) == 0 {
		return ""
	}
	return clip(cardsLine(named), maxRecapChars)
}

type ShownKind string

const (
	ShownCard   ShownKind = "card"
	ShownPanel  ShownKind = "panel"
	ShownButton ShownKind = "button"
	ShownMoved  ShownKind = "moved"
	ShownCart   ShownKind = "cart"
	ShownText   ShownKind = "text" // a recap none of the lines above could be read from
)

// Shown is one thing a reply put in front of the shopper besides its words
type Shown struct {
	Kind      ShownKind
	Name      string
	ProductID uuid.UUID
	Label     string
	Path      string
	Text      string
}

var namedRef = regexp.MustCompile(`^(?:(.+?) )?\(?id ([0-9a-f-]{36})\)?$`)

// ReadRecap reads back what a recap says was shown, for a person looking at the conversation
func ReadRecap(recap string) []Shown {
	recap = strings.TrimSpace(recap)
	if recap == "" {
		return nil
	}
	var out []Shown
	for _, part := range recapParts(recap) {
		switch {
		case strings.HasPrefix(part, recapPanel):
			if s, ok := readNamed(ShownPanel, strings.TrimPrefix(part, recapPanel)); ok {
				out = append(out, s)
			}
		case strings.HasPrefix(part, recapCards):
			for card := range strings.SplitSeq(strings.TrimPrefix(part, recapCards), "; ") {
				if s, ok := readNamed(ShownCard, card); ok {
					out = append(out, s)
				}
			}
		case strings.HasPrefix(part, recapLink):
			rest := strings.TrimPrefix(part, recapLink)
			quoted, err := strconv.QuotedPrefix(rest)
			if err != nil {
				continue
			}
			label, _ := strconv.Unquote(quoted)
			if path, ok := strings.CutPrefix(rest[len(quoted):], " to "); ok {
				out = append(out, Shown{Kind: ShownButton, Label: label, Path: path})
			}
		case strings.HasPrefix(part, recapMoved):
			out = append(out, Shown{Kind: ShownMoved, Path: strings.TrimPrefix(part, recapMoved)})
		case part == recapCart:
			out = append(out, Shown{Kind: ShownCart})
		}
	}
	if len(out) == 0 {
		return []Shown{{Kind: ShownText, Text: recap}}
	}
	return out
}

// The lines start where one of the known starts follows a ". " - a product's own name may hold one
func recapParts(recap string) []string {
	cuts := []int{0}
	for _, start := range []string{recapPanel, recapCards, recapLink + `"`, recapMoved, recapCart} {
		for i := 0; ; {
			j := strings.Index(recap[i:], ". "+start)
			if j < 0 {
				break
			}
			cuts = append(cuts, i+j)
			i += j + 2
		}
	}
	sort.Ints(cuts)
	cuts = slices.Compact(cuts)
	parts := make([]string, 0, len(cuts))
	for k, from := range cuts {
		to := len(recap)
		if k+1 < len(cuts) {
			to = cuts[k+1]
		}
		parts = append(parts, strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(recap[from:to]), ". "), "."))
	}
	return parts
}

func readNamed(kind ShownKind, s string) (Shown, bool) {
	m := namedRef.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Shown{}, false
	}
	id, err := uuid.Parse(m[2])
	if err != nil {
		return Shown{}, false
	}
	return Shown{Kind: kind, Name: strings.TrimSpace(m[1]), ProductID: id}, true
}
