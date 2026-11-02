// Package agenttools is the read-only tool menu the admin chat and the MCP endpoint both serve - the model picks a name and JSON args off a fixed menu and never writes a query, and the channel is pinned server-side, so a tool can only return what the caller may already see
package agenttools

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/sales"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

// What the tools read through - wiring fills every field but Signals, which is nil when the suite isn't addressed
type Deps struct {
	Admin        *admin.Service
	AI           *ai.Service
	Catalog      *catalog.Service
	Channel      *channel.Service
	CMS          *cms.Service
	Currency     *currency.Service
	Customer     *customer.Service
	Giftcard     *giftcard.Service
	Inventory    *inventory.Service
	Locale       *locale.Service
	Menu         *menu.Service
	Order        *order.Service
	Promotion    *promotion.Service
	Review       *review.Service
	Sales        *sales.Service
	Shipping     *shipping.Service
	Subscription *subscription.Service
	Tax          *tax.Service
	URLRewrite   *urlrewrite.Service

	Signals        *signalsclient.Client
	SignalsEnabled bool

	DefaultLocale locale.DefaultLocaleProvider
	Attributes    signalsclient.TrackedAttributes
}

type Service struct{ deps *Deps }

func New(deps Deps) *Service { return &Service{deps: &deps} }

func (deps *Deps) activeLocale(ctx context.Context) string {
	if tag := locale.Active(ctx, deps.DefaultLocale); tag != "" {
		return tag
	}
	return channel.FallbackLocale
}
