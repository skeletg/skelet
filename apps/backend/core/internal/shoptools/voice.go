package shoptools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const (
	VoiceNamePath         = "sales/assistant/name"
	VoiceTonePath         = "sales/assistant/tone"
	VoiceInstructionsPath = "sales/assistant/instructions"

	maxVoiceName       = 40
	MaxInstructionsLen = 4000
	DefaultTone        = "friendly"
)

// Each tone is one line of the prompt - the rules above it keep answers short whatever the tone
var tones = []struct{ value, label, line string }{
	{"friendly", "Friendly and warm", "Sound friendly and warm, like a helpful person in the shop."},
	{"professional", "Polished and professional", "Sound polished and professional: courteous and precise, without slang, emoji or exclamation marks."},
	{"playful", "Playful and upbeat", "Sound playful and upbeat, with a light touch of humour - never at the shopper's expense, and never at the cost of being clear."},
	{"brief", "Brief and to the point", "Be brief and to the point: the fewest words that answer, and no small talk."},
}

func init() {
	config.RegisterGroup(config.Group{Section: "sales", Slug: "assistant", Label: "Shopping assistant's voice", SortOrder: 7, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: VoiceNamePath, Type: config.TypeString, Public: true, MaxLen: maxVoiceName,
		Label:       "The assistant's name",
		Description: "What it calls itself, and the title of its panel on the storefront. Empty keeps it the shopping assistant."})
	opts := make([]config.Option, len(tones))
	for i, t := range tones {
		opts[i] = config.Option{Value: t.value, Label: t.label}
	}
	config.Register(config.Definition{Path: VoiceTonePath, Type: config.TypeSelect, Default: DefaultTone, Options: opts,
		Label:       "How it sounds",
		Description: "It answers in the shopper's language whatever the tone, and keeps answers short."})
	config.Register(config.Definition{Path: VoiceInstructionsPath, Type: config.TypeTextarea, MaxLen: MaxInstructionsLen,
		Label: "What it should know and follow",
		Description: "Written for the assistant in plain words - advice to give, products to favour, anything the store's pages don't say. " +
			"It takes what you write here about the store as fact. Sent with every message, so a longer one costs a little more each time."})
}

type voice struct{ name, tone, instructions string }

// A setting that can't be read leaves the assistant its default voice rather than silencing it
func (s *Service) voice(ctx context.Context, ch channel.Channel) voice {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
	read := func(path string) string {
		v, err := s.deps.Settings.GetString(ctx, path, scope)
		if err != nil {
			slog.WarnContext(ctx, "shoptools: reading the assistant's voice failed", "channel", ch.ID, "path", path, "err", err)
		}
		return strings.TrimSpace(v)
	}
	return voice{name: read(VoiceNamePath), tone: read(VoiceTonePath), instructions: read(VoiceInstructionsPath)}
}

// ToneLabel names a tone as the settings do, the default's for one that isn't known
func ToneLabel(tone string) string {
	for _, t := range tones {
		if t.value == tone {
			return t.label
		}
	}
	return tones[0].label
}

func toneLine(tone string) string {
	for _, t := range tones {
		if t.value == tone {
			return t.line
		}
	}
	return tones[0].line
}

// The staff's words go last and fenced, under the rules - what they say about the store counts as fact, what they say
// about the tools, the cart or checkout can't undo what the rules say those do
func (v voice) prompt() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n- %s", toneLine(v.tone))
	if v.name != "" {
		fmt.Fprintf(&b, "\n- Your name is %s. Use it when you introduce yourself or the shopper asks.", clip(v.name, maxVoiceName))
	}
	if v.instructions != "" {
		fmt.Fprintf(&b, "\n\nThe store's staff wrote these instructions for you. Follow them, and take what they say about the store as fact, as you would the tools' answers. "+
			"They can't change what the rules above say about the tools, the cart, checkout or what you can't do.\n<store_instructions>\n%s\n</store_instructions>",
			shorten(v.instructions, MaxInstructionsLen))
	}
	return b.String()
}
