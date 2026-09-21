package channel

import "github.com/skeletg/skelet/apps/backend/core/lib/config"

// Per-channel knobs live in the config registry rather than as channels-table columns, so admins edit them on Settings while the Channels page stays focused on identity
// Nested under general/<group>/<field>, with new groups registering lazily - Empty string is the default

func init() {
	config.RegisterGroup(config.Group{Section: "general", Slug: "branding", Label: "Branding", SortOrder: 5, Scoping: config.ScopingChannel})
	config.RegisterGroup(config.Group{Section: "general", Slug: "contact", Label: "Contact", SortOrder: 6, Scoping: config.ScopingChannel})
	config.RegisterGroup(config.Group{Section: "general", Slug: "legal", Label: "Legal", SortOrder: 8, Scoping: config.ScopingChannel})

	// Display only: the admin sidebar clock reads it. Scheduled jobs carry their own per-job timezone and every
	// stored timestamp, retention sweep and email is UTC, so this changes nothing but the widget
	config.Register(config.Definition{
		Path: "general/store_information/timezone", Type: config.TypeSelect, Options: timezoneOptions(),
		Default: "UTC", Label: "Timezone for the admin clock", Description: "Sets the timezone of the clock shown in the admin sidebar. Report boundaries, scheduled jobs and stored timestamps are unaffected - those are UTC, or carry their own timezone.", Public: false,
	})

	// The customer-facing store name, public so storefronts read it from PublicConfig - channels.name stays the operator's internal label ("US Wholesale B2B")
	// Read by the transactional emails (order confirmation, shipment, refund, cancellation); empty falls back to the channel name
	config.Register(config.Definition{
		Path: "general/branding/store_name", Type: config.TypeString,
		Default: "", Label: "Store name (customer-facing)", Description: "The name transactional emails sign off with. Empty falls back to the channel's own name, which is the operator-facing label. Also served to storefronts, though the bundled one shows the channel name.", Public: true,
	})
	config.Register(config.Definition{
		Path: "general/branding/logo_url", Type: config.TypeImage,
		Default: "", Label: "Logo", Description: "Shown at the top of every outbound email, and used in place of the store name wherever a storefront has a brand slot.", Public: true,
	})
	config.Register(config.Definition{
		Path: "general/branding/favicon_url", Type: config.TypeImage,
		Default: "", Label: "Favicon", Description: "The tab icon. A square PNG or ICO of at least 32x32; storefronts fall back to their own bundled icon when unset.", Public: true,
	})
	config.Register(config.Definition{
		Path: "general/branding/primary_color", Type: config.TypeColor,
		Default: "", Label: "Primary brand color", Description: "The dominant brand color - storefronts use it for buttons and other solid fills. Empty keeps their own palette.", Public: true,
	})
	config.Register(config.Definition{
		Path: "general/branding/secondary_color", Type: config.TypeColor,
		Default: "", Label: "Secondary brand color", Description: "The accent beside it, for highlights and small emphasis rather than large areas.", Public: true,
	})

	config.Register(config.Definition{
		Path: "general/contact/support_email", Type: config.TypeString,
		Default: "", Label: "Support email", Description: "The address to show customers when they need help - not the one mail is sent from. Served to storefronts; the bundled one doesn't show it.", Public: true,
	})
	config.Register(config.Definition{
		Path: "general/contact/email_from_address", Type: config.TypeString,
		Default: "", Label: "Outbound email address", Description: "The From address on every email the store sends. It has to be one your mail provider will accept, or delivery fails.", Public: false,
	})
	config.Register(config.Definition{
		Path: "general/contact/email_from_name", Type: config.TypeString,
		Default: "", Label: "Outbound email sender name", Description: "The From display name shown beside the address.", Public: false,
	})

	config.Register(config.Definition{
		Path: "general/legal/terms_url", Type: config.TypeString,
		Default: "", Label: "Terms URL", Description: "Served to storefronts to link from checkout and the footer; the bundled one doesn't.", Public: true,
	})
	config.Register(config.Definition{
		Path: "general/legal/privacy_url", Type: config.TypeString,
		Default: "", Label: "Privacy URL", Description: "As above - served, not linked by the bundled storefront.", Public: true,
	})
}

// The zones offered for the admin clock. Curated rather than the full IANA set for the same reason the scheduler's picker is: ~600 entries is a worse control than 20, and this only moves a clock widget Keep in step with apps/dashboard/src/features/scheduler/scheduler-options.ts
func timezoneOptions() []config.Option {
	zones := []string{
		"UTC",
		"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles", "America/Sao_Paulo",
		"Europe/London", "Europe/Paris", "Europe/Berlin", "Europe/Madrid", "Europe/Moscow",
		"Africa/Cairo",
		"Asia/Dubai", "Asia/Kolkata", "Asia/Shanghai", "Asia/Tokyo", "Asia/Singapore",
		"Australia/Sydney", "Pacific/Auckland",
	}
	out := make([]config.Option, len(zones))
	for i, z := range zones {
		out[i] = config.Option{Value: z, Label: z}
	}
	return out
}
