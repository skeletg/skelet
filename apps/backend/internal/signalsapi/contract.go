// Package signalsapi is the contract between core and the Signals API - neutral ground because wiring assembles the server while graph/admin mints the token, and wiring already imports graph/admin
package signalsapi

// Audience names the service in a core-minted token, and a token for anything else is refused there
const Audience = "signals"

// Slugs are the permissions the Signals API checks, and a token carries only these - the list grows as suites move onto that surface, and a slug missing here is a resolver that refuses every caller
var Slugs = []string{
	"experiments.view",
	"experiments.manage",
	"lens.view",
	"settings.manage", // the event-destination screens
	"promotions.view",
	"promotions.manage", // segments are administered under promotions
	"customers.view",    // Synapse - customer behaviour, profiles, audiences
	"emails.view",       // the Pulse engagement rollup, which reads the spine
}
