package wiring

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type switchSetting struct {
	open bool
	err  error
}

func (s switchSetting) GetBool(context.Context, string, config.Scope) (bool, error) {
	return s.open, s.err
}

func (s switchSetting) GetInt(context.Context, string, config.Scope) (int64, error) {
	return 0, s.err
}

func (s switchSetting) GetString(context.Context, string, config.Scope) (string, error) {
	return "", s.err
}

func TestTheStorefrontSwitchDecidesWhoGetsTools(t *testing.T) {
	ch := channel.Channel{ID: uuid.New(), PublicURL: "https://shop.example"}
	for name, tc := range map[string]struct {
		setting switchSetting
		ch      channel.Channel
		want    int
	}{
		"switched on":       {switchSetting{open: true}, ch, 0},
		"switched off":      {switchSetting{}, ch, http.StatusNotFound},
		"unreadable switch": {switchSetting{err: errors.New("valkey down")}, ch, http.StatusServiceUnavailable},
		"no public address": {switchSetting{open: true}, channel.Channel{ID: uuid.New()}, http.StatusNotFound},
	} {
		tools, refused := storefrontTools(shoptools.New(shoptools.Deps{Settings: tc.setting}))(context.Background(), tc.ch, "ShopBot")
		switch {
		case tc.want == 0 && (refused != nil || tools == nil):
			t.Errorf("%s: refused %+v", name, refused)
		case tc.want != 0 && (refused == nil || refused.Status != tc.want):
			t.Errorf("%s: got %+v, want status %d", name, refused, tc.want)
		}
	}
}

func TestTheBridgeReadsDataAndTypedErrors(t *testing.T) {
	answer := func(body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if q, _ := io.ReadAll(r.Body); len(q) == 0 || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("the bridge sent %q", q)
			}
			_, _ = io.WriteString(w, body)
		})
	}
	ctx := context.Background()
	var out struct {
		Cart struct{ ID string } `json:"cart"`
	}
	if err := (frontGraphQL{answer(`{"data":{"cart":{"id":"c1"}}}`)}).Do(ctx, "query { cart }", nil, &out); err != nil || out.Cart.ID != "c1" {
		t.Errorf("data = %+v, %v", out, err)
	}
	err := (frontGraphQL{answer(`{"errors":[{"message":"that code has expired","extensions":{"code":"PROMOTION_CODE_EXPIRED"}}],"data":null}`)}).Do(ctx, "mutation { x }", nil, nil)
	if gql, ok := errors.AsType[*shoptools.Error](err); !ok || gql.Code != "PROMOTION_CODE_EXPIRED" || gql.Message != "that code has expired" {
		t.Errorf("typed error = %#v", err)
	}
	if err := (frontGraphQL{answer(`<html>oops</html>`)}).Do(ctx, "query { x }", nil, &out); err == nil {
		t.Error("a body that isn't GraphQL's should be an error")
	}
}
