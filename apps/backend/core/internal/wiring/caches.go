package wiring

import (
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

type valkeyCacheDeps struct {
	fx.In
	Valkey     *redis.Client // primary - only the promotion pub/sub bus which stores nothing
	Cache      CacheRedis    // evictable - the read-through value caches
	Catalog    *catalog.Service
	URLRewrite *urlrewrite.Service
	Menu       *menu.Service
	Promotion  *promotion.Service
}

// Every Valkey namespace the domain services own - the reads front queries the storefront makes on nearly every page, and the promotion bus is the odd one out, carrying invalidations between pods rather than values
func WireValkeyCaches(d valkeyCacheDeps) {
	vk := d.Cache.Client

	d.Catalog.SetChannelPathCache(cache.New(vk, "chanpath")) // channel pretty-URL → product id, hit by the `[...slug]` catch-all
	d.URLRewrite.SetCache(cache.New(vk, "urlrewrite"))       // positive AND negative hits - without it, bot traffic and typo URLs each pay a Postgres round-trip
	d.Menu.SetCache(cache.New(vk, "menu"))                   // one entry per slug x store x locale - the storefront resolver reads the namespace the menu service invalidates on write

	d.Promotion.Engine().SetCacheBus(promotion.NewCacheBus(d.Valkey)) // pub/sub on the primary - without it, disabling a bad discount only took effect on the pod that served the edit
}
