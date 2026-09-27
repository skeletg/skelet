// Package consentkind is the list of consent kinds a customer can answer, shared by core's registry and the suite's segment rules
package consentkind

const (
	MarketingEmail     = "marketing_email"
	MarketingSMS       = "marketing_sms"
	TransactionalEmail = "transactional_email"
)

type Kind struct {
	Slug         string
	Name         string
	Description  string
	Purpose      string
	Channel      string
	Required     bool
	DefaultOptIn bool // the answer for a customer with no stored row - GDPR: MUST be false for marketing
}

var kinds = []Kind{
	{
		Slug: MarketingEmail, Name: "Marketing emails",
		Description: "Newsletters, promotions, and product announcements.",
		Purpose:     "marketing", Channel: "email",
	},
	{
		Slug: MarketingSMS, Name: "Marketing SMS",
		Description: "Promotions, cart reminders, and product news by text message.",
		Purpose:     "marketing", Channel: "sms",
	},
	{
		Slug: TransactionalEmail, Name: "Transactional emails",
		Description: "Order confirmations, shipping updates, password reset and account security messages. Required while your account is active.",
		Purpose:     "transactional", Channel: "email",
		Required: true, DefaultOptIn: true,
	},
}

func All() []Kind { return append([]Kind(nil), kinds...) }

func Find(slug string) (Kind, bool) {
	for _, k := range kinds {
		if k.Slug == slug {
			return k, true
		}
	}
	return Kind{}, false
}
