// Package signalsapi is the contract between core and the Signals API - neutral ground because wiring assembles the server while graph/admin mints the token, and wiring already imports graph/admin
package signalsapi

// Audience names the service in a core-minted token, and a token for anything else is refused there
const Audience = "signals"

// CoreAudience is the other direction - a token the suite mints for core's own surfaces with the same service secret
// The storefront API's per-IP limiter doesn't count a caller that carries one
const CoreAudience = "skelet-core"

// ServiceTokenHeader carries that token on an HTTP request - its own header, Authorization stays the shopper's session
const ServiceTokenHeader = "X-Skelet-Service-Token"

// Slugs are the permissions the Signals API checks, and a token carries only these - the list grows as suites move onto that surface, and a slug missing here is a resolver that refuses every caller
var Slugs = []string{
	"experiments.view",
	"experiments.manage",
	"lens.view",
	"settings.manage", // the event-destination screens
	"promotions.view",
	"promotions.manage", // segments are administered under promotions
	"customers.view",    // Synapse - customer behaviour, profiles, audiences
	"emails.view",       // Pulse - the email builder, and the engagement rollup off the spine
	"emails.manage",
	"flows.view",
	"flows.manage",
	"campaigns.view",
	"campaigns.manage",
}
