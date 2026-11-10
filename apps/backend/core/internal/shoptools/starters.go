package shoptools

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

const (
	maxFollowUps     = 3
	maxFollowUpChars = 80
)

// The model ends a reply with [[next: one | two | three]] - the follow-ups ride in the same answer, so offering them
// costs no extra round with the model
var followUpsLine = regexp.MustCompile(`(?s)\s*\[\[next:\s*(.*?)\]\]\s*$`)

// The model marks a reply the store's information couldn't answer, for the store to see where its pages and catalog fall short
var unansweredMark = regexp.MustCompile(`[ \t]*\[\[\s*unanswered\s*\]\][ \t]*\n?`)

// splitUnanswered takes that mark out of a reply, wherever the model put it
func splitUnanswered(reply string) (string, bool) {
	if !unansweredMark.MatchString(reply) {
		return reply, false
	}
	return strings.TrimSpace(unansweredMark.ReplaceAllString(reply, "")), true
}

// splitFollowUps takes the suggested next messages off the end of a reply
func splitFollowUps(reply string) (string, []string) {
	m := followUpsLine.FindStringSubmatchIndex(reply)
	if m == nil {
		return reply, nil
	}
	var next []string
	for opt := range strings.SplitSeq(reply[m[2]:m[3]], "|") {
		opt = strings.TrimSpace(strings.Trim(strings.TrimSpace(opt), `"'“”‘’`))
		if opt = clip(opt, maxFollowUpChars); opt != "" && !slices.Contains(next, opt) && len(next) < maxFollowUps {
			next = append(next, opt)
		}
	}
	return strings.TrimSpace(reply[:m[0]]), next
}

// PageInfo is what the shopper's page is, for the assistant's opening suggestions - a product, a product list, a
// search, an information page, the home page, or anything else - and the name it goes by
type PageInfo struct {
	Kind  string
	Title string
}

// PageContext reads the shopper's page without asking the model anything, so opening the assistant costs nothing
func (s *Service) PageContext(ctx context.Context, ch channel.Channel, page string) (PageInfo, error) {
	if !s.AssistantOpen(ctx, ch) {
		return PageInfo{}, ErrAssistantUnavailable
	}
	conv := &conversation{svc: s, ch: ch, page: pagePath(page)}
	k := call{svc: s, ch: ch}
	if conv.page == "" {
		return PageInfo{Kind: "other"}, nil
	}
	at, err := conv.place(ctx, k)
	switch {
	case err != nil:
		return PageInfo{}, err
	case at.list != nil && at.list.Kind == "search" && at.list.Name == "":
		return PageInfo{Kind: "other"}, nil
	case at.list != nil && at.list.Kind == "search":
		return PageInfo{Kind: "search", Title: at.list.Name}, nil
	case at.list != nil:
		return PageInfo{Kind: "listing", Title: at.list.Name}, nil
	case at.home:
		return PageInfo{Kind: "home"}, nil
	case at.route == nil:
		return PageInfo{Kind: "other"}, nil
	case at.route.Kind == "PRODUCT":
		found, err := lookupProducts(ctx, k, []string{at.route.EntityID})
		if err != nil {
			return PageInfo{}, err
		}
		if len(found) > 0 {
			return PageInfo{Kind: "product", Title: found[0].Name}, nil
		}
	case at.route.Kind == "CMS_PAGE":
		p, err := cmsPage(ctx, k, at.route)
		if err != nil {
			return PageInfo{}, err
		}
		if p != nil {
			return PageInfo{Kind: "page", Title: p.title}, nil
		}
	}
	return PageInfo{Kind: "other"}, nil
}
