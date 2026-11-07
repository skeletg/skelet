package tracking

// CatalogEntry is a platform skelet ships a plugin for, whether or not this build compiled it in - so the dashboard
// can show the whole choice, and say how to add the ones that are missing
type CatalogEntry struct {
	Kind   string
	Label  string
	Plugin string // the slug to enable: skelet cli plugin enable <slug>
}

// FirstParty lists every destination the first-party plugins register. catalog_test.go holds it to the plugins
// themselves, so adding or renaming one fails the tests until this list follows
var FirstParty = []CatalogEntry{
	{Kind: "ga4", Label: "Google Analytics 4", Plugin: "tracking-ga4"},
	{Kind: "google_ads", Label: "Google Ads conversions", Plugin: "tracking-google-ads"},
	{Kind: "google_ads_audience", Label: "Google Ads Customer Match", Plugin: "tracking-google-ads"},
	{Kind: "meta_capi", Label: "Meta (Facebook, Instagram)", Plugin: "tracking-meta"},
	{Kind: "bing_uet", Label: "Microsoft Advertising (Bing)", Plugin: "tracking-bing"},
	{Kind: "openai", Label: "OpenAI Ads (ChatGPT)", Plugin: "tracking-openai"},
	{Kind: "cj", Label: "CJ Affiliate", Plugin: "tracking-cj"},
	{Kind: "awin", Label: "Awin", Plugin: "tracking-awin"},
	{Kind: "klaviyo", Label: "Klaviyo", Plugin: "tracking-klaviyo"},
	{Kind: "attentive", Label: "Attentive", Plugin: "tracking-attentive"},
	{Kind: "webhook", Label: "Webhook (signed JSON)", Plugin: "tracking-webhook"},
}
