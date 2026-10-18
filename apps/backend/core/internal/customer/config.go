package customer

import "github.com/skeletg/skelet/apps/backend/core/lib/config"

// When true, Authenticate refuses a session for an unverified customer with ErrEmailNotVerified - defaults false to preserve the storefront flow
const pathRequireEmailVerification = "general/security/require_email_verification"

// Registered by the country domain - read here by path, as storeName reads branding, to parse a profile phone typed without its + prefix
const pathDefaultCountry = "general/country/default"

func init() {
	config.Register(config.Definition{
		Path:    pathRequireEmailVerification,
		Type:    config.TypeBool,
		Default: "false",
		Label:   "Require verified email to sign in",
		Public:  false,
		// Authenticate resolves it against the channel the customer is signing in to
		Scoping: config.ScopingChannel,
	})
}
