package catalog

// Per-domain service types carved off the Service god-struct - Each embeds *core (shared deps + cross-cutting helpers) and hangs its domain's methods off that receiver (see the matching *_service.go file) - The facade Service embeds all of them so callers keep using catalogSvc.<Method> unchanged. productService is declared in product_service.go (the first one extracted)
type (
	categoryService          struct{ *core }
	collectionService        struct{ *core }
	optionService            struct{ *core }
	mediaService             struct{ *core }
	bundleService            struct{ *core }
	groupedService           struct{ *core }
	downloadService          struct{ *core }
	subscriptionTermsService struct{ *core }
	kitService               struct{ *core }
	listingService           struct{ *core }
)
