package review

import "github.com/skeletg/skelet/internal/config"

// Policy knobs in the config registry, so admins flip them on /settings without a redeploy - read at request time on the eligibility and submit paths
// require_authentication defaults true; setting it false stops the eligibility resolver returning NOT_AUTHENTICATED, but the mutation still needs customer auth, since reviews.customer_id is NOT NULL - real anonymous submission would need a guest-author scheme and a migration
// require_verified_purchase defaults true and needs a confirmed or fulfilled order line for the SKU. votes/require_authentication defaults true, and false accepts anonymous votes debounced by a cookie token, one per browser per review
const (
	pathRequireAuthentication       = "reviews/policy/require_authentication"
	pathRequireVerifiedPurchase     = "reviews/policy/require_verified_purchase"
	pathRequireAuthenticationToVote = "reviews/votes/require_authentication"
)

func init() {
	config.RegisterSection(config.Section{Slug: "reviews", Label: "Reviews", SortOrder: 5})
	config.RegisterGroup(config.Group{
		Section:   "reviews",
		Slug:      "policy",
		Label:     "Submission Policy",
		SortOrder: 1,
	})
	config.RegisterGroup(config.Group{
		Section:   "reviews",
		Slug:      "votes",
		Label:     "Vote Policy",
		SortOrder: 2,
	})
	config.Register(config.Definition{
		Path:    pathRequireAuthentication,
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Require sign-in",
	})
	config.Register(config.Definition{
		Path:    pathRequireVerifiedPurchase,
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Require verified purchase",
	})
	config.Register(config.Definition{
		Path:    pathRequireAuthenticationToVote,
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Require sign-in to vote on reviews",
	})
}
