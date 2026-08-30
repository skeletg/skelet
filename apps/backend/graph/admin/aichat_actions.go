package admin // chat only PROPOSES an action through a widget in its reply - the operator confirms and this dispatcher runs it

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skeletg/skelet/internal/auth"
)

// Whitelist only, and every case validates its args then routes through the existing gated resolver method rather than the service - that's what keeps the model from reaching anything the operator couldn't
func dispatchChatAction(ctx context.Context, r *mutationResolver, action, argsJSON string) (ok bool, msg string, err error) {
	var adminID string
	if claims, c := auth.AdminFromContext(ctx); c && claims != nil {
		adminID = claims.AdminID.String()
	}
	defer func() {
		slog.InfoContext(ctx, "chat: action", "action", action, "admin", adminID, "ok", ok, "err", err)
	}()
	switch action {
	case "reply_to_review":
		var a struct {
			ReviewID string `json:"review_id"`
			Body     string `json:"body"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return false, "", errInvalidInput("arguments", nil)
		}
		if strings.TrimSpace(a.ReviewID) == "" || strings.TrimSpace(a.Body) == "" {
			return false, "", fmt.Errorf("review_id and body are required")
		}
		if _, err := r.ReplyToReview(ctx, a.ReviewID, a.Body); err != nil { // re-checks reviews.moderate inline, so this path is gated exactly like the dashboard's
			return false, "", err
		}
		return true, "Reply posted to the review.", nil
	default:
		return false, "", fmt.Errorf("unknown action %q", action)
	}
}
