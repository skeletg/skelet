package ai

import "github.com/skeletg/skelet/internal/config"

const (
	cfgEnabled           = "ai/llm/enabled"
	cfgProvider          = "ai/llm/provider"
	cfgAPIKey            = "ai/llm/api_key"
	cfgModel             = "ai/llm/model"
	cfgVisionModel       = "ai/llm/vision_model"
	cfgBaseURL           = "ai/llm/base_url"
	cfgTools             = "ai/llm/tools"
	cfgAllowCustomerData = "ai/llm/allow_customer_data"
	cfgHistoryRetention  = "ai/llm/history_retention_days"
)

// "openai" is any OpenAI-compatible Chat Completions endpoint - OpenAI, Azure, Gemini's OpenAI endpoint, OpenRouter, Groq, Together, and local servers (Ollama / LM Studio / vLLM), picked apart by base_url
const (
	providerAnthropic = "anthropic"
	providerOpenAI    = "openai"
)

const defaultModel = "claude-sonnet-4-6" // matches the default provider - switching providers means setting the matching model too, e.g. gpt-4o

func init() { // only this package registers "ai", so no dedup guard - RegisterSection panics on duplicate
	config.RegisterSection(config.Section{Slug: "ai", Label: "AI", SortOrder: 70})
	config.RegisterGroup(config.Group{Section: "ai", Slug: "llm", Label: "LLM provider", SortOrder: 1})
	config.Register(config.Definition{
		Path:    cfgEnabled,
		Type:    config.TypeBool,
		Default: "false",
		Label:   "Enable AI content assistant",
	})
	config.Register(config.Definition{
		Path:    cfgProvider,
		Type:    config.TypeSelect,
		Default: providerAnthropic,
		Label:   "Provider",
		Options: []config.Option{
			{Value: providerAnthropic, Label: "Anthropic (Claude)"},
			{Value: providerOpenAI, Label: "OpenAI-compatible (OpenAI, Azure, Gemini, OpenRouter, Ollama, …)"},
		},
	})
	config.Register(config.Definition{
		Path:    cfgAPIKey,
		Type:    config.TypeSecret, // encrypted at rest + masked in the admin API
		Default: "",
		Label:   "API key",
	})
	config.Register(config.Definition{
		Path:    cfgModel,
		Type:    config.TypeString,
		Default: defaultModel,
		Label:   "Model (e.g. claude-sonnet-4-6, gpt-4o, gemini-2.0-flash)",
	})
	config.Register(config.Definition{
		Path:    cfgVisionModel,
		Type:    config.TypeString,
		Default: "",
		Label:   "Vision model for alt-text (blank = use Model)",
	})
	config.Register(config.Definition{
		Path:    cfgBaseURL,
		Type:    config.TypeString,
		Default: "",
		Label:   "API base URL (blank = provider default; set for Azure/OpenRouter/Ollama/local)",
	})
	config.Register(config.Definition{
		Path:    cfgTools,
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Let the assistant query data via tools (turn off for models without tool/function calling)",
	})
	config.Register(config.Definition{
		Path:    cfgAllowCustomerData,
		Type:    config.TypeBool,
		Default: "true",
		Label:   "Allow the assistant to read individual customer data (profiles, records). Turn off to keep personal data out of the AI provider.",
	})
	config.Register(config.Definition{
		Path:    cfgHistoryRetention,
		Type:    config.TypeInt,
		Default: "90",
		Label:   "Chat history retention (days; 0 = keep forever)",
	})
}
