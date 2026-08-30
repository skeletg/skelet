package analytics

import "strings"

// The result of User-Agent bot classification - IsBot is false for empty or human UAs, Name is a readable agent name, and Category buckets it for reporting as ai, search, social, seo, monitoring, scraper or other
type Bot struct {
	IsBot    bool
	Name     string
	Category string
}

type botSignature struct{ marker, name, category string }

// Checked in order, first contained marker winning, so more specific markers must precede broader ones - "google-extended" before "googlebot"
// Curated from the public crawler lists the analytics tools use, with the AI block answering "which LLMs crawl me"
var botSignatures = []botSignature{
	{"gptbot", "GPTBot (OpenAI)", "ai"},
	{"oai-searchbot", "OAI-SearchBot (OpenAI)", "ai"},
	{"chatgpt-user", "ChatGPT-User (OpenAI)", "ai"},
	{"claudebot", "ClaudeBot (Anthropic)", "ai"},
	{"claude-web", "Claude-Web (Anthropic)", "ai"},
	{"claude-user", "Claude-User (Anthropic)", "ai"},
	{"anthropic-ai", "Anthropic-AI", "ai"},
	{"google-extended", "Google-Extended (Gemini)", "ai"},
	{"googleother", "GoogleOther", "ai"},
	{"perplexitybot", "PerplexityBot", "ai"},
	{"perplexity-user", "Perplexity-User", "ai"},
	{"bytespider", "Bytespider (TikTok)", "ai"},
	{"ccbot", "CCBot (Common Crawl)", "ai"},
	{"diffbot", "Diffbot", "ai"},
	{"amazonbot", "Amazonbot", "ai"},
	{"applebot-extended", "Applebot-Extended", "ai"},
	{"meta-externalagent", "Meta-ExternalAgent", "ai"},
	{"facebookbot", "FacebookBot", "ai"},
	{"cohere-ai", "Cohere-AI", "ai"},
	{"youbot", "YouBot", "ai"},
	{"ai2bot", "AI2Bot", "ai"},
	{"omgili", "Omgili (Webz)", "ai"},
	{"timpibot", "Timpibot", "ai"},
	{"imagesiftbot", "ImageSiftBot", "ai"},
	{"googlebot", "Googlebot", "search"},
	{"adsbot-google", "AdsBot-Google", "search"},
	{"mediapartners-google", "Mediapartners-Google", "search"},
	{"bingbot", "Bingbot", "search"},
	{"adidxbot", "AdIdxBot (Bing Ads)", "search"},
	{"slurp", "Yahoo! Slurp", "search"},
	{"duckduckbot", "DuckDuckBot", "search"},
	{"duckassistbot", "DuckAssistBot", "search"},
	{"baiduspider", "Baidu Spider", "search"},
	{"yandexbot", "YandexBot", "search"},
	{"yandex", "Yandex", "search"},
	{"sogou", "Sogou", "search"},
	{"exabot", "Exabot", "search"},
	{"seznambot", "SeznamBot", "search"},
	{"petalbot", "PetalBot (Huawei)", "search"},
	{"applebot", "Applebot", "search"},
	{"yeti", "Naver Yeti", "search"},
	{"mojeekbot", "MojeekBot", "search"},
	{"facebookexternalhit", "Facebook", "social"},
	{"twitterbot", "Twitterbot", "social"},
	{"linkedinbot", "LinkedInBot", "social"},
	{"pinterest", "Pinterest", "social"},
	{"slackbot", "Slackbot", "social"},
	{"telegrambot", "TelegramBot", "social"},
	{"whatsapp", "WhatsApp", "social"},
	{"discordbot", "Discordbot", "social"},
	{"redditbot", "Redditbot", "social"},
	{"skypeuripreview", "Skype URI Preview", "social"},
	{"embedly", "Embedly", "social"},
	{"ahrefsbot", "AhrefsBot", "seo"},
	{"semrushbot", "SemrushBot", "seo"},
	{"mj12bot", "MJ12bot (Majestic)", "seo"},
	{"dotbot", "DotBot (Moz)", "seo"},
	{"rogerbot", "rogerbot (Moz)", "seo"},
	{"screaming frog", "Screaming Frog", "seo"},
	{"blexbot", "BLEXBot", "seo"},
	{"dataforseo", "DataForSeoBot", "seo"},
	{"barkrowler", "Barkrowler", "seo"},
	{"serpstatbot", "serpstatbot", "seo"},
	{"uptimerobot", "UptimeRobot", "monitoring"},
	{"pingdom", "Pingdom", "monitoring"},
	{"statuscake", "StatusCake", "monitoring"},
	{"datadog", "Datadog", "monitoring"},
	{"newrelicpinger", "New Relic", "monitoring"},
	{"gtmetrix", "GTmetrix", "monitoring"},
	{"site24x7", "Site24x7", "monitoring"},
	{"chrome-lighthouse", "Lighthouse", "monitoring"},
	{"lighthouse", "Lighthouse", "monitoring"},
	{"headlesschrome", "Headless Chrome", "scraper"},
	{"phantomjs", "PhantomJS", "scraper"},
	{"selenium", "Selenium", "scraper"},
	{"playwright", "Playwright", "scraper"},
	{"puppeteer", "Puppeteer", "scraper"},
	{"python-requests", "python-requests", "scraper"},
	{"python-urllib", "python-urllib", "scraper"},
	{"aiohttp", "aiohttp", "scraper"},
	{"scrapy", "Scrapy", "scraper"},
	{"go-http-client", "Go HTTP client", "scraper"},
	{"node-fetch", "node-fetch", "scraper"},
	{"axios", "axios", "scraper"},
	{"okhttp", "OkHttp", "scraper"},
	{"apache-httpclient", "Apache HttpClient", "scraper"},
	{"libwww-perl", "libwww-perl", "scraper"},
	{"java/", "Java", "scraper"},
	{"curl/", "curl", "scraper"},
	{"wget", "Wget", "scraper"},
}

// Flag a UA that self-identifies as automated but matches no known signature, named "Other bot" - conservative, since real browser UAs contain none of these tokens
var genericBotMarkers = []string{"bot", "crawler", "spider", "crawl", "scanner", "fetcher", "+http", "http-client"}

// Classifies a User-Agent - the first matching known signature wins, else a generic marker yields an unnamed "Other bot", else the UA is treated as human
func DetectBot(ua string) Bot {
	low := strings.ToLower(strings.TrimSpace(ua))
	if low == "" {
		return Bot{}
	}
	for _, sig := range botSignatures {
		if strings.Contains(low, sig.marker) {
			return Bot{IsBot: true, Name: sig.name, Category: sig.category}
		}
	}
	for _, m := range genericBotMarkers {
		if strings.Contains(low, m) {
			return Bot{IsBot: true, Name: "Other bot", Category: "other"}
		}
	}
	return Bot{}
}
