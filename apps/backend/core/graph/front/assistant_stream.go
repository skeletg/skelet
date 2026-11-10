package front

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type streamStep struct {
	Tool   string `json:"tool"`
	Detail string `json:"detail"`
}

type streamError struct {
	Code string `json:"code"`
}

type streamCart struct {
	ID string `json:"id"`
}

type streamLine struct {
	Step  *streamStep           `json:"step,omitempty"`
	Cart  *streamCart           `json:"cart,omitempty"`
	Reply *model.AssistantReply `json:"reply,omitempty"`
	Error *streamError          `json:"error,omitempty"`
}

type streamInput struct {
	Messages     []*model.AssistantMessage `json:"messages"`
	CartID       *string                   `json:"cartId"`
	Page         *string                   `json:"page"`
	Conversation *string                   `json:"conversationId"`
}

type askFunc func(ctx context.Context, in streamInput, notify shoptools.Notify) (*model.AssistantReply, error)

// AssistantStream answers askShopAssistant's input as NDJSON: a step line as each tool call starts, a cart line when the turn makes a guest a cart, then the reply or the error code the mutation would have answered with
func AssistantStream(shop *shoptools.Service) http.Handler {
	return assistantStream(func(ctx context.Context, in streamInput, notify shoptools.Notify) (*model.AssistantReply, error) {
		return askAssistant(ctx, shop, in.Messages, in.CartID, in.Page, in.Conversation, notify)
	})
}

func assistantStream(ask askFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		// A JSON body makes a browser ask first, so another site can't post here from its visitors' browsers and spend the store's allowance
		if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
			http.Error(w, "send the body as application/json", http.StatusUnsupportedMediaType)
			return
		}
		var in streamInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "a JSON body with messages is required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		flush := http.NewResponseController(w)
		enc := json.NewEncoder(w)
		send := func(line streamLine) {
			if err := enc.Encode(line); err == nil {
				_ = flush.Flush()
			}
		}
		reply, err := ask(r.Context(), in, shoptools.Notify{
			Step: func(s shoptools.Step) { send(streamLine{Step: &streamStep{Tool: s.Tool, Detail: s.Detail}}) },
			Cart: func(id uuid.UUID) { send(streamLine{Cart: &streamCart{ID: id.String()}}) },
		})
		if err != nil {
			code := apperr.CodeOf(err)
			if code == "" {
				slog.ErrorContext(r.Context(), "assistant stream failed", "err", err)
				code = "INTERNAL"
			}
			send(streamLine{Error: &streamError{Code: code}})
			return
		}
		send(streamLine{Reply: reply})
	})
}
