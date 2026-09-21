package front

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// The front schema is the real gate on what an anonymous caller may ask for - resolvers only answer fields the schema declares, and front output types autobind to graph/admin/model, so adding one line to a .graphqls is all it takes to publish an admin-only column This walks the schema the server actually serves and flags every output field whose NAME looks sensitive, deliberately over-matching - each hit must be written down below, so a newly published field fails until someone looks at it

// Matched case-insensitively as substrings of the field name - broad on purpose
var sensitiveTokens = []string{
	"password", "secret", "token", "hash", "apikey",
	"note", "email", "internal", "moderat",
	"cost", "margin", "supplier", "credit", "superuser", "payout",
}

// Field -> why a public caller may see it, keyed Type.field so two types can differ on the same name
var acknowledgedFrontExposures = map[string]string{
	// The caller's own account, reachable only through the authenticated me/auth paths
	"Customer.email":                   "the caller's own address, on their own profile",
	"Customer.isEmailVerified":         "the caller's own verification state",
	"CustomerAuthPayload.accessToken":  "the session being issued to this caller",
	"CustomerAuthPayload.refreshToken": "the session being issued to this caller",

	// Notes the customer wrote, coming back to the customer who wrote them
	"Cart.note":          "the shopper's own cart note",
	"Order.customerNote": "the note the buyer left on their own order",

	// Store credit - all three hang off the caller's own customer, order or redemption
	"Customer.storeCreditBalance":            "the caller's own balance, and the resolver re-checks obj.ID against the session",
	"LoyaltyRedeemResult.storeCreditBalance": "the balance left after the caller's own redemption",
	"Order.storeCreditAmount":                "how much credit was applied to the caller's own order",

	// Handed to the browser on purpose - these are the public half of a gateway handshake, scoped to one payment
	"Order.paymentClientSecret": "gateway client secret, meant for the browser that completes the payment",

	// Published author metadata
	"BlogAuthor.email": "author contact, published deliberately with the post",
}

// The same, for fields a plugin's SDL publishes - enforced while its type is in the schema, inert
// when it isn't, and kept in core because the test can't import a plugin's package
var acknowledgedPluginExposures = map[string]string{
	// b2b - all resolve the company from the session, never an argument, so no id aims at another account
	"MyB2BCompany.creditLimitCents": "the caller's own company credit terms",
	"MyB2BCompany.creditUsedCents":  "the caller's own company credit terms",
	"MyB2BQuote.note":               "the buyer's own note on their own quote request",
	"MyB2BQuote.sellerNote":         "the seller's pricing reply, written FOR the buyer",
	"MyB2BCompanyOrder.memberEmail": "which colleague placed the order, on the caller's own company history, company admins only",
}

func TestFrontSchemaPublishesNothingSensitiveUnacknowledged(t *testing.T) {
	fields := frontOutputFields(t)

	var unacknowledged []string
	for _, key := range fields {
		_, field, _ := strings.Cut(key, ".")
		if !looksSensitive(field) {
			continue
		}
		reason, ok := acknowledgedFrontExposures[key]
		if !ok {
			reason, ok = acknowledgedPluginExposures[key]
		}
		if !ok {
			unacknowledged = append(unacknowledged, key)
			continue
		}
		if reason == "" {
			t.Errorf("%s is acknowledged with no reason", key)
		}
	}
	sort.Strings(unacknowledged)
	for _, key := range unacknowledged {
		t.Errorf("front schema publishes %s, whose name reads as sensitive on a public surface - remove it from "+
			"the front schema, or record in acknowledgedFrontExposures why an anonymous or non-owning caller may see it", key)
	}
}

// A stale entry means the list has stopped describing the schema, which is how the next real one slips past
func TestAcknowledgedFrontExposureListIsCurrent(t *testing.T) {
	present := map[string]bool{}
	for _, key := range frontOutputFields(t) {
		present[key] = true
	}

	for key := range acknowledgedFrontExposures {
		if !present[key] {
			t.Errorf("acknowledgedFrontExposures lists %s, which the front schema no longer publishes - drop the entry", key)
			continue
		}
		if _, field, _ := strings.Cut(key, "."); !looksSensitive(field) {
			t.Errorf("acknowledgedFrontExposures lists %s, which no longer matches any sensitive token - drop the entry", key)
		}
	}

	// Only checked while the type is in the schema - absent means not installed, not stale
	for key, reason := range acknowledgedPluginExposures {
		if reason == "" {
			t.Errorf("acknowledgedPluginExposures lists %s with no reason", key)
		}
		if _, field, _ := strings.Cut(key, "."); !looksSensitive(field) {
			t.Errorf("acknowledgedPluginExposures lists %s, which no longer matches any sensitive token - drop the entry", key)
		}
	}
}

func looksSensitive(field string) bool {
	lower := strings.ToLower(field)
	for _, tok := range sensitiveTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

// Every readable field on the front surface as "Type.field", from BOTH the .graphqls source and the schema baked into generated.go - the union, so the check can only ever ask for one acknowledgement too many, never miss one Source alone would miss a schema location the globs below don't cover; generated alone only updates once gqlgen has run, so a field published in this commit would slip past until codegen Root operation fields are operation NAMES, not data, so resetPassword and verifyEmail are not exposures; input objects are what a caller sends rather than what it reads, and gqlparser keeps those in a separate kind anyway
func frontOutputFields(t *testing.T) []string {
	t.Helper()

	seen := map[string]bool{}
	collect := func(schema *ast.Schema) {
		for _, def := range schema.Types {
			if strings.HasPrefix(def.Name, "__") {
				continue
			}
			if def.Kind != ast.Object && def.Kind != ast.Interface {
				continue
			}
			switch def.Name {
			case "Query", "Mutation", "Subscription":
				continue
			}
			for _, f := range def.Fields {
				if strings.HasPrefix(f.Name, "__") {
					continue
				}
				seen[def.Name+"."+f.Name] = true
			}
		}
	}
	collect(loadFrontSchemaSource(t))
	collect(NewExecutableSchema(Config{}).Schema())

	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	if len(out) < 200 {
		t.Fatalf("found only %d output fields - the schema is not loading", len(out))
	}
	sort.Strings(out)
	return out
}

// Mirrors the schema globs in gqlgen.front.yml - core SDL plus each Tier 1 plugin's storefront SDL The plugins/* wildcard picks up a newly
// installed plugin on its own, and anything these miss still arrives through the generated half of the union
var frontSchemaGlobs = []string{
	"../schema/front/*.graphqls",
	"../../plugins/*/graphql/front/*.graphqls",
}

func loadFrontSchemaSource(t *testing.T) *ast.Schema {
	t.Helper()

	var sources []*ast.Source
	for _, glob := range frontSchemaGlobs {
		paths, err := filepath.Glob(glob)
		if err != nil {
			t.Fatalf("glob %s: %v", glob, err)
		}
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			sources = append(sources, &ast.Source{Name: path, Input: string(body)})
		}
	}
	if len(sources) < 10 {
		t.Fatalf("found only %d schema files across %v - wrong working directory?", len(sources), frontSchemaGlobs)
	}

	schema, gqlErr := gqlparser.LoadSchema(sources...)
	if gqlErr != nil {
		t.Fatalf("load front schema: %v", gqlErr)
	}
	return schema
}
