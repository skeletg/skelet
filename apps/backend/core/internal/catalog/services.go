package catalog

// Per-domain service types carved off the Service god-struct - Each embeds *core (shared deps + cross-cutting helpers) and hangs its domain's methods off that receiver (see the matching *_service.go file) - The facade Service embeds all of them so callers keep using catalogSvc.<Method> unchanged. productService is declared in product_service.go (the first one extracted)
// Each one's repo field shadows core.repo with just the roles it reads; core's helpers, and anything opening a tx, still go through core.repo
type (
	categoryService struct {
		*core
		repo interface {
			CategoryRepository
			ProductRepository
			ProductContentRepository
		}
	}
	collectionService struct {
		*core
		repo interface {
			CollectionRepository
			ProductRepository
			ProductContentRepository
		}
	}
	optionService struct {
		*core
		repo interface {
			OptionRepository
			ProductRepository
		}
	}
	mediaService struct {
		*core
		repo interface {
			MediaRepository
			ProductRepository
		}
	}
	bundleService struct {
		*core
		repo interface {
			BundleRepository
			ProductRepository
		}
	}
	groupedService struct {
		*core
		repo interface {
			GroupedRepository
			ProductRepository
		}
	}
	downloadService struct {
		*core
		repo interface {
			DownloadRepository
			ProductRepository
		}
	}
	subscriptionTermsService struct {
		*core
		repo interface {
			SubscriptionTermsRepository
			ProductRepository
		}
	}
	kitService struct {
		*core
		repo interface {
			KitRepository
			ProductRepository
		}
	}
	listingService struct {
		*core
		repo interface {
			ListingRepository
			ProductRepository
		}
	}
)
