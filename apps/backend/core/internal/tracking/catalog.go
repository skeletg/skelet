package tracking

// CatalogEntry is a platform skelet publishes a plugin for, whether or not this build compiled it in - so the dashboard
// can show the whole choice, and say how to add the ones that are missing
type CatalogEntry struct {
	Kind   string
	Label  string
	Plugin string // the slug, and the repository's name: skelet cli plugin add github.com/skeletg/<slug>
}

// FirstParty lists the destinations of the plugins skelet publishes, each in its own repository
var FirstParty = []CatalogEntry{
	{Kind: "ga4", Label: "Google Analytics 4", Plugin: "tracking-ga4"},
	{Kind: "google_ads", Label: "Google Ads conversions", Plugin: "tracking-google-ads"},
	{Kind: "google_ads_audience", Label: "Google Ads Customer Match", Plugin: "tracking-google-ads"},
	{Kind: "meta_capi", Label: "Meta (Facebook, Instagram)", Plugin: "tracking-meta"},
}
