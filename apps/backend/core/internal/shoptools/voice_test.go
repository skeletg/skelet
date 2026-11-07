package shoptools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestTheStoresVoiceReachesThePrompt(t *testing.T) {
	model := &fakeModel{}
	svc := New(Deps{GraphQL: &fakeGraphQL{}, Carts: &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}, Model: model, Settings: fakeSettings{open: true, strings: map[string]string{
		VoiceNamePath: " Ava ", VoiceTonePath: "professional", VoiceInstructionsPath: "Our jeans run small - suggest a size up.",
	}}})
	if _, err := svc.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Your name is Ava.", "without slang, emoji or exclamation marks", "<store_instructions>\nOur jeans run small - suggest a size up.\n</store_instructions>"} {
		if !strings.Contains(model.system, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if i, j := strings.Index(model.system, "Never offer what you've just done"), strings.Index(model.system, "<store_instructions>"); i < 0 || j < i {
		t.Error("the store's instructions should come after the rules")
	}
}

func TestADefaultVoiceAddsNoInstructions(t *testing.T) {
	plain := assistantPrompt(shop, voice{})
	if strings.Contains(plain, "store_instructions") || strings.Contains(plain, "Your name is") || !strings.Contains(plain, toneLine(DefaultTone)) {
		t.Errorf("an unset voice should be the friendly default and nothing else:\n%s", plain[len(plain)-300:])
	}
	if toneLine("shouty") != toneLine(DefaultTone) || ToneLabel("shouty") != "Friendly and warm" {
		t.Error("an unknown tone should fall back to the default")
	}
}

func TestAnUnreadableVoiceStillAnswers(t *testing.T) {
	model := &fakeModel{}
	svc := New(Deps{GraphQL: &fakeGraphQL{}, Carts: &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}, Model: model, Settings: voiceDown{fakeSettings{open: true}}})
	if _, err := svc.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); err != nil {
		t.Fatalf("a voice setting that can't be read silenced the assistant: %v", err)
	}
	if !strings.Contains(model.system, toneLine(DefaultTone)) {
		t.Error("want the default voice")
	}
}

type voiceDown struct{ fakeSettings }

func (voiceDown) GetString(context.Context, string, config.Scope) (string, error) {
	return "", errors.New("settings down")
}
