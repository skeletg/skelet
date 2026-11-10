package ai

import (
	"context"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type models map[string]string

func (m models) GetBool(context.Context, string, config.Scope) (bool, error) { return true, nil }
func (m models) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return m[path], nil
}

func TestTheQuickModelAnswersOnlyWhereAskedFor(t *testing.T) {
	ctx := context.Background()
	svc := NewService(models{cfgModel: "claude-opus-5-5", QuickModelPath: "claude-haiku-5-5"})
	if got := svc.ModelName(ctx); got != "claude-opus-5-5" {
		t.Errorf("an ordinary call runs on %q", got)
	}
	if got := svc.ModelName(Quick(ctx)); got != "claude-haiku-5-5" {
		t.Errorf("a quick call runs on %q", got)
	}
	if got := NewService(models{cfgModel: "gpt-4o"}).ModelName(Quick(ctx)); got != "gpt-4o" {
		t.Errorf("a quick call with none set runs on %q - want the main model", got)
	}
}
