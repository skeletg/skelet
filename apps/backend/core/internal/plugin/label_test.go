package plugin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

type countingLabeler struct{ calls int }

func (l *countingLabeler) Label(context.Context, sdk.LabelRequest) (sdk.LabelResult, error) {
	l.calls++
	return sdk.LabelResult{}, nil
}

// Off for one channel means no label bought for that channel's shipment - Quote already gated per channel, Label read the default scope
func TestLabelHonoursAChannelKillSwitch(t *testing.T) {
	off := uuid.New()
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, ch uuid.UUID) bool { return ch != off }

	l := &countingLabeler{}
	if _, err := instrumentedLabel(context.Background(), "carrier", l, nil, shipping.LabelRequest{ChannelID: off}); err == nil {
		t.Error("label bought on a channel with the plugin switched off")
	}
	if _, err := instrumentedLabel(context.Background(), "carrier", l, nil, shipping.LabelRequest{ChannelID: uuid.New()}); err != nil {
		t.Errorf("enabled channel refused: %v", err)
	}
	if l.calls != 1 {
		t.Errorf("labeler calls: got %d, want 1", l.calls)
	}
}
