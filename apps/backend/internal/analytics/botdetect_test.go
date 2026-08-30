package analytics

import "testing"

func TestDetectBot(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		wantBot  bool
		wantName string
		wantCat  string
	}{
		{"empty", "", false, "", ""},
		{
			"chrome human",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
			false, "", "",
		},
		{
			"safari iphone human",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
			false, "", "",
		},
		{
			"googlebot",
			"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			true, "Googlebot", "search",
		},
		{
			"bingbot",
			"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
			true, "Bingbot", "search",
		},
		{
			"gptbot",
			"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; GPTBot/1.2; +https://openai.com/gptbot",
			true, "GPTBot (OpenAI)", "ai",
		},
		{
			"claudebot",
			"Mozilla/5.0 (compatible; ClaudeBot/1.0; +claudebot@anthropic.com)",
			true, "ClaudeBot (Anthropic)", "ai",
		},
		{
			"google-extended ranks as gemini ai not search",
			"Mozilla/5.0 (compatible; Google-Extended)",
			true, "Google-Extended (Gemini)", "ai",
		},
		{
			"perplexity",
			"Mozilla/5.0 (compatible; PerplexityBot/1.0; +https://perplexity.ai/bot)",
			true, "PerplexityBot", "ai",
		},
		{
			"ahrefs seo",
			"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)",
			true, "AhrefsBot", "seo",
		},
		{
			"curl scraper",
			"curl/8.4.0",
			true, "curl", "scraper",
		},
		{
			"unknown self-identified bot",
			"SomeRandomCrawler/1.0 (+http://example.com)",
			true, "Other bot", "other",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectBot(tt.ua)
			if got.IsBot != tt.wantBot {
				t.Fatalf("IsBot = %v, want %v", got.IsBot, tt.wantBot)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Category != tt.wantCat {
				t.Errorf("Category = %q, want %q", got.Category, tt.wantCat)
			}
		})
	}
}
