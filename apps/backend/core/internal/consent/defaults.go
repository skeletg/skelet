package consent

// Core platform consent kinds - Domain packages that need their own (e.g. a future "order-status SMS" kind) register
// from their own init() using the same API

func init() {
	Register(Definition{
		Slug:         "marketing_email",
		Name:         "Marketing emails",
		Description:  "Newsletters, promotions, and product announcements.",
		Purpose:      PurposeMarketing,
		Channel:      ChannelEmail,
		Required:     false,
		DefaultOptIn: false, // GDPR-compliant default for marketing
	})

	Register(Definition{
		Slug:         "marketing_sms",
		Name:         "Marketing SMS",
		Description:  "Promotions, cart reminders, and product news by text message.",
		Purpose:      PurposeMarketing,
		Channel:      ChannelSMS,
		Required:     false,
		DefaultOptIn: false, // GDPR-compliant default for marketing
	})

	Register(Definition{
		Slug:         "transactional_email",
		Name:         "Transactional emails",
		Description:  "Order confirmations, shipping updates, password reset and account security messages. Required while your account is active.",
		Purpose:      PurposeTransactional,
		Channel:      ChannelEmail,
		Required:     true,
		DefaultOptIn: true,
	})
}
