package promotion

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// Turns a pattern string into a batch of unique-ish codes, square-bracket placeholders are replaced with random characters and everything else is literal [A] is an uppercase letter, [N] a digit, [*] alphanumeric - so "WELCOME-[N][N][N]" yields "WELCOME-481" and "SPRING-[A][A][N][N]" yields "SPRING-XQ47" Prefix and suffix wrap the whole pattern, and Validate() catches malformed patterns at the admin write path
type CodeGenerator struct {
	pattern string
	prefix  string
	suffix  string
}

func NewCodeGenerator(pattern, prefix, suffix string) *CodeGenerator {
	return &CodeGenerator{pattern: pattern, prefix: prefix, suffix: suffix}
}

// Ensures the pattern is parseable and produces non-empty output - the admin mutation calls it before Batch so a typo doesn't burn a generation slot
func (g *CodeGenerator) Validate() error {
	if g.pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	// Walk the pattern counting placeholders - with zero, every generated code would be identical and the bulk insert would dedupe to one row, so reject up front
	placeholders := 0
	runes := []rune(g.pattern)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '[' {
			continue
		}
		if i+2 >= len(runes) || runes[i+2] != ']' {
			return fmt.Errorf("malformed placeholder at position %d", i)
		}
		switch runes[i+1] {
		case 'A', 'N', '*':
			placeholders++
		default:
			return fmt.Errorf("unknown placeholder [%c] at position %d", runes[i+1], i)
		}
		i += 2
	}
	if placeholders == 0 {
		return fmt.Errorf("pattern must contain at least one [A], [N], or [*] placeholder")
	}
	return nil
}

// How many distinct codes the pattern can produce - float64 since a long pattern overflows any int
func (g *CodeGenerator) Space() float64 {
	space := 1.0
	runes := []rune(g.pattern)
	for i := 0; i+2 < len(runes); i++ {
		if runes[i] != '[' || runes[i+2] != ']' {
			continue
		}
		switch runes[i+1] {
		case 'A':
			space *= 26
		case 'N':
			space *= 10
		case '*':
			space *= 36
		}
		i += 2
	}
	return space
}

// One generated code - codes are uppercased on output
func (g *CodeGenerator) One() (string, error) {
	var b strings.Builder
	if g.prefix != "" {
		b.WriteString(strings.ToUpper(g.prefix))
	}
	runes := []rune(g.pattern)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '[' {
			// literals too - ApplyCode uppercases what the shopper types, so a lowercase code could never match
			b.WriteString(strings.ToUpper(string(runes[i])))
			continue
		}
		if i+2 >= len(runes) || runes[i+2] != ']' {
			return "", fmt.Errorf("malformed placeholder at position %d", i)
		}
		ch, err := randChar(runes[i+1])
		if err != nil {
			return "", err
		}
		b.WriteRune(ch)
		i += 2
	}
	if g.suffix != "" {
		b.WriteString(strings.ToUpper(g.suffix))
	}
	return b.String(), nil
}

// Produces n codes for BulkInsertCodes, which dedupes via ON CONFLICT, so a small collision rate is fine - for 10k codes from a pattern with 6 alphanumeric placeholders, around 2bn possibilities, collisions are vanishingly rare
func (g *CodeGenerator) Batch(n int) []string {
	out := make([]string, 0, n)
	seen := make(map[string]struct{}, n)
	for i := 0; i < n*2 && len(out) < n; i++ {
		// Generate up to 2x to handle in-batch dupes - the bulk insert handles cross-batch and cross-rule ones
		c, err := g.One()
		if err != nil {
			continue
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func randChar(kind rune) (rune, error) {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const alnum = letters + digits
	var alphabet string
	switch kind {
	case 'A':
		alphabet = letters
	case 'N':
		alphabet = digits
	case '*':
		alphabet = alnum
	default:
		return 0, fmt.Errorf("unknown placeholder kind %c", kind)
	}
	// crypto/rand for unguessable codes - these are tied to money, and math/rand's predictable seed would be a coupon-mining vector
	buf := make([]byte, 1)
	for {
		if _, err := rand.Read(buf); err != nil {
			return 0, err
		}
		// Reject-sample to avoid modulo bias
		if int(buf[0]) < (256/len(alphabet))*len(alphabet) {
			return rune(alphabet[int(buf[0])%len(alphabet)]), nil
		}
	}
}
