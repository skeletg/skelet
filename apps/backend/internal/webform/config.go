package webform

import "github.com/skeletg/skelet/internal/config"

// Runtime config for the custom form builder, editable at admin /settings - the "cms" section comes from internal/cms and this adds the "forms" group under it
// Labeled "Custom Forms" rather than "Forms" to keep it distinct from Pulse's marketing signup widget - CAPTCHA is optional: leave the provider "none" and forms rely on honeypot plus rate limiting
func init() {
	config.RegisterGroup(config.Group{Section: "cms", Slug: "forms", Label: "Custom Forms", SortOrder: 5})
	config.Register(config.Definition{
		Path:    CfgEnabled,
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Enable custom forms on the storefront",
	})
	config.Register(config.Definition{
		Path:    CfgCaptchaProvider,
		Type:    config.TypeSelect,
		Default: "none",
		Label:   "CAPTCHA provider for forms that require it",
		Options: []config.Option{
			{Value: "none", Label: "None"},
			{Value: "turnstile", Label: "Cloudflare Turnstile"},
			{Value: "hcaptcha", Label: "hCaptcha"},
		},
	})
	config.Register(config.Definition{
		Path:    CfgCaptchaSiteKey,
		Type:    config.TypeString,
		Default: "",
		Label:   "CAPTCHA site key (public)",
	})
	config.Register(config.Definition{
		Path:    CfgCaptchaSecretKey,
		Type:    config.TypeSecret, // encrypted at rest + masked in the admin API
		Default: "",
		Label:   "CAPTCHA secret key",
	})
}
