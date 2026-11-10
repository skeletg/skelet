package ai

import (
	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Registered here rather than in aitrace, after the provider and the embeddings, so the settings read in that order
func init() {
	config.RegisterGroup(config.Group{Section: "ai", Slug: "langfuse", Label: "Langfuse", SortOrder: 9})
	config.Register(config.Definition{Path: aitrace.EnabledPath, Type: config.TypeBool, Default: "false",
		Label: "Send AI calls to Langfuse",
		Description: "Each shopping assistant turn and admin chat message as a trace, with every model call's tokens and timing and every tool it ran. " +
			"What shoppers wrote and what the assistant answered goes only for conversations the store keeps - the shopper agreed to analytics, with Keep the shopping assistant's conversations on - and everything else as numbers alone."})
	config.Register(config.Definition{Path: aitrace.HostPath, Type: config.TypeString, Default: aitrace.DefaultHost,
		Label:       "Langfuse address",
		Description: "https://cloud.langfuse.com for the EU cloud, https://us.cloud.langfuse.com for the US one, or your own Langfuse's address."})
	config.Register(config.Definition{Path: aitrace.PublicKeyPath, Type: config.TypeString, Label: "Public key", Description: "pk-lf-... from the Langfuse project's settings."})
	config.Register(config.Definition{Path: aitrace.SecretKeyPath, Type: config.TypeSecret, Label: "Secret key", Description: "sk-lf-... from the same place."})
}
