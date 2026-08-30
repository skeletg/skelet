import { Navigate } from 'react-router-dom'
import { lazyPage } from './lazy'
import {
  LayoutDashboardIcon,
  UsersIcon,
  UserCogIcon,
  ShoppingCartIcon,
  PackageIcon,
  PlugZapIcon,
  ScrollTextIcon,
  SettingsIcon,
  StoreIcon,
  SquareTerminalIcon,
  SatelliteDishIcon,
  WebhookIcon,
  CodeIcon,
  FileStackIcon,
  ClockIcon,
  MegaphoneIcon,
  ActivityIcon,
  WarehouseIcon,
  ArrowUpDownIcon,
} from 'lucide-react'
import {
  registerDetailRoute,
  registerNavNode,
} from '@/nav/registry'

// Core nav plugin: registers every built-in sidebar entry and detail
// route against the nav registry - Imported once by App.tsx - side
// effects run at module load - Add new core pages here; 3rd-party
// pages drop a file under `src/plugins/<name>/plugin.tsx` and call
// the same register* functions

registerNavNode({
  title: 'Dashboard',
  path: '/',
  icon: <LayoutDashboardIcon />,
  element: lazyPage(() => import('@/features/dashboard/Home'), 'Home'),
})

registerNavNode({
  id: 'admins',
  title: 'Admins',
  icon: <UserCogIcon />,
  children: [
    { title: 'All admins', path: '/admins', element: lazyPage(() => import('@/features/admins/AdminsList'), 'AdminsList'), permission: 'admins.view' },
    { title: 'Roles', path: '/admins/roles', element: lazyPage(() => import('@/features/admins/roles/RolesList'), 'RolesList'), permission: 'admins.view' },
    { title: 'Permissions', path: '/admins/permissions', element: lazyPage(() => import('@/features/admins/permissions/PermissionCatalog'), 'PermissionCatalog'), permission: 'admins.view' },
  ],
})

registerNavNode({
  id: 'customers',
  title: 'Customers',
  icon: <UsersIcon />,
  children: [
    { title: 'All customers', path: '/customers', element: lazyPage(() => import('@/features/customers/CustomersList'), 'CustomersList'), permission: 'customers.view' },
    { title: 'Groups', path: '/customers/groups', element: lazyPage(() => import('@/features/customers/CustomerGroupsList'), 'CustomerGroupsList'), permission: 'customers.view' },
    { title: 'Segments', path: '/segments', element: lazyPage(() => import('@/features/segments/SegmentsListPage'), 'SegmentsListPage'), permission: 'promotions.view' },
    { title: 'Companies', path: '/b2b/companies', element: lazyPage(() => import('@/features/b2b/CompaniesPage'), 'CompaniesPage'), permission: 'b2b.view', feature: 'b2b' },
    { title: 'B2B quotes', path: '/b2b/quotes', element: lazyPage(() => import('@/features/b2b/B2BQuotesPage'), 'B2BQuotesPage'), permission: 'b2b.view', feature: 'b2b' },
  ],
})

registerNavNode({
  id: 'orders',
  title: 'Orders',
  icon: <ShoppingCartIcon />,
  children: [
    // Pending / Fulfilled aren't separate pages - they're just values of the
    // orders list's Status column filter (in the table toolbar)
    { title: 'All orders', path: '/orders', element: lazyPage(() => import('@/features/orders/OrdersList'), 'OrdersList'), permission: 'orders.view' },
    { title: 'Carts', path: '/carts', element: lazyPage(() => import('@/features/carts/CartsList'), 'CartsList'), permission: 'orders.view' },
  ],
})

registerNavNode({
  id: 'catalog',
  title: 'Catalog',
  icon: <PackageIcon />,
  children: [
    {
      title: 'Products',
      path: '/products',
      element: lazyPage(() => import('@/features/catalog/ProductsList'), 'ProductsList'),
      permission: 'catalog.view',
    },
    {
      title: 'Categories',
      path: '/categories',
      element: lazyPage(() => import('@/features/catalog/CategoriesPage'), 'CategoriesPage'),
      permission: 'catalog.view',
    },
    {
      title: 'Collections',
      path: '/collections',
      element: lazyPage(() => import('@/features/catalog/CollectionsPage'), 'CollectionsPage'),
      permission: 'catalog.view',
    },
    {
      title: 'Product Types',
      path: '/catalog/product-types',
      element: lazyPage(() => import('@/features/catalog/ProductTypesList'), 'ProductTypesList'),
      permission: 'catalog.view',
    },
    {
      title: 'Attributes',
      path: '/catalog/attributes',
      element: lazyPage(() => import('@/features/catalog/AttributesList'), 'AttributesList'),
      permission: 'catalog.view',
    },
    {
      title: 'Search merchandising',
      path: '/search-merch',
      element: lazyPage(() => import('@/features/searchmerch/SearchMerchPage'), 'SearchMerchPage'),
      permission: 'searchmerch.manage',
    },
  ],
})

// Inventory - Locations + the Low stock report grouped under one entry, since the
// sidebar renders group children as sub-items and Low stock stops being a standalone catalog page
registerNavNode({
  id: 'inventory',
  title: 'Inventory',
  icon: <WarehouseIcon />,
  children: [
    {
      title: 'Locations',
      path: '/inventory/locations',
      element: lazyPage(() => import('@/features/inventory/InventoryLocationsList'), 'InventoryLocationsList'),
      permission: 'inventory.view',
    },
    {
      title: 'Low stock',
      path: '/inventory/low-stock',
      element: lazyPage(() => import('@/features/inventory/LowStockList'), 'LowStockList'),
      permission: 'inventory.view',
    },
  ],
})

// Marketing - the broader commerce-marketing tools (promotions, gift cards, subscriptions, reviews) - The
// Pulse/Lens/Synapse analytics suite moved out to its own "Signals" group above - Paths are unchanged
registerNavNode({
  id: 'marketing',
  title: 'Marketing',
  icon: <MegaphoneIcon />,
  subgroups: [
    {
      items: [
        {
          title: 'Promotions',
          path: '/promotions',
          element: lazyPage(() => import('@/features/promotions/PromotionsHome'), 'PromotionsHome'),
          permission: 'promotions.view',
        },
        {
          title: 'Gift cards',
          path: '/gift-cards',
          element: lazyPage(() => import('@/features/gift-cards/GiftCardsList'), 'GiftCardsList'),
          permission: 'giftcards.view',
        },
        {
          title: 'Subscriptions',
          path: '/subscriptions',
          element: lazyPage(() => import('@/features/subscriptions/SubscriptionsList'), 'SubscriptionsList'),
          permission: 'subscriptions.view',
        },
        {
          title: 'Reviews',
          path: '/reviews',
          element: lazyPage(() => import('@/features/reviews/ReviewsHome'), 'ReviewsHome'),
          permission: 'reviews.view',
        },
        {
          title: 'Product Q&A',
          path: '/qna',
          element: lazyPage(() => import('@/features/qna/QuestionsList'), 'QuestionsList'),
          permission: 'qna.view',
        },
        {
          title: 'Event destinations',
          path: '/destinations',
          element: lazyPage(() => import('@/features/destinations/DestinationsPage'), 'DestinationsPage'),
          permission: 'settings.manage',
          feature: 'signals',
        },
        // Fallback entry point to the Emails hub when the Signals suite (and with it the Pulse hub) is switched off - transactional
        // templates, the library, and blocks stay editable - With Signals on the hub is reached via Pulse, so this entry hides itself
        {
          title: 'Email templates',
          path: '/emails',
          element: lazyPage(() => import('@/features/emails/EmailsHub'), 'EmailsHub'),
          permission: 'emails.view',
          feature: '!signals',
        },
      ],
    },
  ],
})


registerNavNode({
  id: 'content-seo',
  title: 'Content & SEO',
  icon: <FileStackIcon />,
  subgroups: [
    {
      items: [
        // CMS hub: pages, page models, menus and the blog all live behind /cms (CmsHome); their list pages are
        // route-only (detailLeaves), reached from the hub cards - mirroring how the Signals products work
        {
          title: 'CMS',
          path: '/cms',
          element: lazyPage(() => import('@/features/cms/CmsHome'), 'CmsHome'),
          permission: 'cms.view',
        },
        {
          title: 'Media Library',
          path: '/media',
          element: lazyPage(() => import('@/features/media/MediaLibrary'), 'MediaLibrary'),
          permission: 'media.view',
        },
        {
          title: 'URL Rewrites',
          path: '/seo/url-rewrites',
          element: lazyPage(() => import('@/features/catalog/UrlRewritesListPage'), 'UrlRewritesListPage'),
          permission: 'urlrewrite.view',
        },
        {
          title: 'Link map',
          path: '/seo/link-map',
          element: lazyPage(() => import('@/features/seo/SiteLinkMap'), 'SiteLinkMap'),
          permission: 'urlrewrite.view',
        },
        {
          title: 'Facet indexing',
          path: '/seo/facets',
          element: lazyPage(() => import('@/features/seo/FacetSeoPage'), 'FacetSeoPage'),
          permission: 'urlrewrite.view',
        },
      ],
    },
  ],
})

// Signals - skelet's customer engagement & intelligence suite: the three
// single-word products (Pulse / Lens / Synapse) that turn customer signals into
// action - emitting them (email), capturing them (web behaviour), and stitching
// them into a person (profiles) - Their inner pages stay route-only (detailLeaves)
registerNavNode({
  id: 'signals',
  title: 'Signals',
  icon: <ActivityIcon />,
  children: [
    {
      title: 'Pulse',
      subtitle: 'Email marketing & automated flows',
      path: '/pulse',
      feature: 'signals',
      element: lazyPage(() => import('@/features/pulse/PulseHome'), 'PulseHome'),
      permission: 'emails.view',
    },
    {
      title: 'Lens',
      subtitle: 'On-site behaviour & heatmaps',
      path: '/lens',
      feature: 'signals',
      element: lazyPage(() => import('@/features/lens/LensHome'), 'LensHome'),
      permission: 'lens.view',
    },
    {
      title: 'Synapse',
      subtitle: 'Customer profiles & journeys',
      path: '/synapse',
      feature: 'signals',
      element: lazyPage(() => import('@/features/synapse/SynapseHome'), 'SynapseHome'),
      permission: 'customers.view',
    },
    {
      title: 'Prism',
      subtitle: 'Frontend A/B experiments',
      path: '/prism',
      feature: 'signals',
      element: lazyPage(() => import('@/features/prism/PrismHome'), 'PrismHome'),
      permission: 'experiments.view',
    },
  ],
})

registerNavNode({
  id: 'developer',
  title: 'Developer',
  icon: <CodeIcon />,
  children: [
    {
      title: 'Integrations',
      path: '/integrations',
      icon: <PlugZapIcon />,
      element: lazyPage(() => import('@/features/integrations/IntegrationsList'), 'IntegrationsList'),
      permission: 'admins.manage',
    },
    {
      title: 'Audit log',
      path: '/audit-log',
      icon: <ScrollTextIcon />,
      element: lazyPage(() => import('@/features/audit/AuditLogPage'), 'AuditLogPage'),
      permission: 'audit.view',
    },
    {
      title: 'Webhooks',
      path: '/webhooks',
      icon: <WebhookIcon />,
      element: lazyPage(() => import('@/features/webhooks/WebhooksList'), 'WebhooksList'),
      permission: 'webhooks.view',
    },
    {
      title: 'Remote providers',
      path: '/remote-providers',
      icon: <SatelliteDishIcon />,
      element: lazyPage(
        () => import('@/features/remote-providers/RemoteProvidersList'),
        'RemoteProvidersList',
      ),
      permission: 'remote_providers.view',
    },
    {
      title: 'Scheduled jobs',
      path: '/scheduled-jobs',
      icon: <ClockIcon />,
      element: lazyPage(() => import('@/features/scheduler/ScheduledJobsList'), 'ScheduledJobsList'),
      permission: 'scheduler.view',
    },
    {
      // Reports on processes, datastores and delivery, so it sits with the other infrastructure pages - and it is NOT
      // feature-gated: the subsystems it covers are the ones every install runs, whether or not Signals is enabled
      title: 'System health',
      path: '/system-health',
      icon: <ActivityIcon />,
      element: lazyPage(
        () => import('@/features/system-health/SystemHealthPage'),
        'SystemHealthPage',
      ),
      permission: 'settings.manage',
    },
    {
      title: 'Playground',
      path: '/playground',
      icon: <SquareTerminalIcon />,
      actionId: 'playground', // opens over the current page rather than routing away from it
      permission: 'developer.playground',
    },
  ],
})

registerNavNode({
  id: 'channels',
  title: 'Channels',
  icon: <StoreIcon />,
  children: [
    {
      title: 'Channels',
      path: '/channels',
      element: lazyPage(() => import('@/features/channels/ChannelsList'), 'ChannelsList'),
      permission: 'channels.view',
    },
    {
      title: 'Locales',
      path: '/locales',
      element: lazyPage(() => import('@/features/locales/LocalesList'), 'LocalesList'),
      permission: 'locales.view',
    },
    {
      title: 'Currencies',
      path: '/currencies',
      element: lazyPage(() => import('@/features/currencies/CurrenciesList'), 'CurrenciesList'),
      permission: 'currencies.view',
    },
    {
      title: 'Countries',
      path: '/countries',
      element: lazyPage(() => import('@/features/countries/CountriesList'), 'CountriesList'),
      permission: 'countries.view',
    },
  ],
})

// Bulk data transfer - its own top-level entry rather than a Settings child, because moving a catalog in or
// out is daily merchandiser work, not configuration. One page for every resource the backend registry declares
registerNavNode({
  title: 'Import & export',
  path: '/data',
  icon: <ArrowUpDownIcon />,
  element: lazyPage(() => import('@/features/data/DataHub'), 'DataHub'),
  permission: 'data.view',
})

// Top-level Settings - General first (header-scope-driven config + bespoke channel editors when
// a channel is scoped), then the commerce configuration pages (tax, shipping - absorbed from the
// removed Commerce group), then the technical pages
//   Search Engine → /search-engine (bespoke per-attribute editor +
//              reindex action; doesn't fit the generic field renderer)
// The channel detail page deep-links to /settings with `?channelId=…`
// to pre-select the channel
registerNavNode({
  id: 'settings',
  title: 'Settings',
  icon: <SettingsIcon />,
  children: [
    {
      title: 'General',
      path: '/settings',
      element: lazyPage(() => import('@/features/settings/SettingsPage'), 'SettingsPage'),
      permission: 'config.view',
    },
    {
      title: 'Tax rules',
      path: '/tax',
      element: lazyPage(() => import('@/features/tax/TaxRulesTable'), 'TaxRulesTable'),
      permission: 'settings.view',
    },
    {
      title: 'Shipping zones',
      path: '/shipping',
      element: lazyPage(() => import('@/features/shipping/ShippingZonesList'), 'ShippingZonesList'),
      permission: 'settings.view',
    },
    {
      title: 'Shipping boxes',
      path: '/shipping/boxes',
      element: lazyPage(() => import('@/features/shipping/ShippingBoxesPage'), 'ShippingBoxesPage'),
      permission: 'settings.manage',
    },
    {
      title: 'Search Engine',
      path: '/search-engine',
      element: lazyPage(() => import('@/features/search/SearchSettings'), 'SearchSettings'),
      permission: 'catalog.view',
    },
    {
      title: 'Cache',
      path: '/cache',
      element: lazyPage(() => import('@/features/cache/CacheAdminPage'), 'CacheAdminPage'),
      permission: 'cache.view',
    },
  ],
})

// Router-only routes (no sidebar entry) - Dynamic detail pages live here
const detailLeaves = [
  { title: 'Admin detail', path: '/admins/:id', element: lazyPage(() => import('@/features/admins/AdminDetail'), 'AdminDetail') },
  { title: 'New role', path: '/admins/roles/new', element: lazyPage(() => import('@/features/admins/roles/RoleEditPage'), 'RoleEditPage') },
  { title: 'Edit role', path: '/admins/roles/:id/edit', element: lazyPage(() => import('@/features/admins/roles/RoleEditPage'), 'RoleEditPage') },
  { title: 'Role detail', path: '/admins/roles/:id', element: lazyPage(() => import('@/features/admins/roles/RoleDetail'), 'RoleDetailPage') },
  { title: 'Attribute detail', path: '/catalog/attributes/:id', element: lazyPage(() => import('@/features/catalog/AttributeDetail'), 'AttributeDetail') },
  // Reviews tabs all render the tabbed ReviewsHome shell (deep-linkable)
  { title: 'Review questions', path: '/reviews/questions', element: lazyPage(() => import('@/features/reviews/ReviewsHome'), 'ReviewsHome'), permission: 'reviews.view' },
  { title: 'Review question', path: '/reviews/questions/:id', element: lazyPage(() => import('@/features/reviews/ReviewQuestionDetail'), 'ReviewQuestionDetailPage'), permission: 'reviews.view' },
  { title: 'Review categories', path: '/reviews/categories', element: lazyPage(() => import('@/features/reviews/ReviewsHome'), 'ReviewsHome'), permission: 'reviews.view' },
  { title: 'Review moderation tags', path: '/reviews/tags', element: lazyPage(() => import('@/features/reviews/ReviewsHome'), 'ReviewsHome'), permission: 'reviews.view' },
  { title: 'Review insights', path: '/reviews/insights', element: lazyPage(() => import('@/features/reviews/ReviewsHome'), 'ReviewsHome'), permission: 'reviews.view' },
  // Old split routes now fold into the merged hub
  { title: 'Pending reviews', path: '/reviews/pending', element: <Navigate to="/reviews" replace />, permission: 'reviews.view' },
  { title: 'All reviews', path: '/reviews/all', element: <Navigate to="/reviews" replace />, permission: 'reviews.view' },
  { title: 'Review detail', path: '/reviews/:id', element: lazyPage(() => import('@/features/reviews/ReviewDetail'), 'ReviewDetail') },
  { title: 'Product type detail', path: '/catalog/product-types/:id', element: lazyPage(() => import('@/features/catalog/ProductTypesList'), 'ProductTypesList') },
  { title: 'New customer', path: '/customers/new', element: lazyPage(() => import('@/features/customers/CustomerCreate'), 'CustomerCreate'), permission: 'customers.manage' },
  { title: 'Customer detail', path: '/customers/:id', element: lazyPage(() => import('@/features/customers/CustomerDetail'), 'CustomerDetail'), permission: 'customers.view' },
  { title: 'Customer group detail', path: '/customers/groups/:id', element: lazyPage(() => import('@/features/customers/CustomerGroupDetail'), 'CustomerGroupDetailPage'), permission: 'customers.view' },
  { title: 'Integration detail', path: '/integrations/:id', element: lazyPage(() => import('@/features/integrations/IntegrationDetail'), 'IntegrationDetail') },
  { title: 'New collection', path: '/collections/new', element: lazyPage(() => import('@/features/catalog/CollectionsPage'), 'CollectionsPage') },
  { title: 'Collection detail', path: '/collections/:id', element: lazyPage(() => import('@/features/catalog/CollectionsPage'), 'CollectionsPage') },
  { title: 'Category', path: '/categories/:id', element: lazyPage(() => import('@/features/catalog/CategoriesPage'), 'CategoriesPage') },
  { title: 'New order', path: '/orders/new', element: lazyPage(() => import('@/features/orders/DraftOrderCreate'), 'DraftOrderCreate'), permission: 'orders.update' },
  { title: 'Order detail', path: '/orders/:id', element: lazyPage(() => import('@/features/orders/OrderDetail'), 'OrderDetail') },
  { title: 'Cart detail', path: '/carts/:id', element: lazyPage(() => import('@/features/carts/CartDetail'), 'CartDetail'), permission: 'orders.view' },
  { title: 'New product', path: '/products/new', element: lazyPage(() => import('@/features/catalog/ProductCreate'), 'ProductCreate') },
  { title: 'Product detail', path: '/products/:id', element: lazyPage(() => import('@/features/catalog/ProductDetail'), 'ProductDetail') },
  { title: 'Channel detail', path: '/channels/:id', element: lazyPage(() => import('@/features/channels/ChannelDetail'), 'ChannelDetail') },
  { title: 'New inventory location', path: '/inventory/locations/new', element: lazyPage(() => import('@/features/inventory/InventoryLocationsList'), 'InventoryLocationsList') },
  { title: 'Edit inventory location', path: '/inventory/locations/:id', element: lazyPage(() => import('@/features/inventory/InventoryLocationsList'), 'InventoryLocationsList') },
  { title: 'New locale', path: '/locales/new', element: lazyPage(() => import('@/features/locales/LocaleForm'), 'LocaleCreatePage') },
  { title: 'Edit locale', path: '/locales/:code', element: lazyPage(() => import('@/features/locales/LocaleForm'), 'LocaleEditPage') },
  { title: 'New currency', path: '/currencies/new', element: lazyPage(() => import('@/features/currencies/CurrencyForm'), 'CurrencyCreatePage') },
  { title: 'Edit currency', path: '/currencies/:code', element: lazyPage(() => import('@/features/currencies/CurrencyForm'), 'CurrencyEditPage') },
  { title: 'New country', path: '/countries/new', element: lazyPage(() => import('@/features/countries/CountryForm'), 'CountryCreatePage') },
  { title: 'Edit country', path: '/countries/:code', element: lazyPage(() => import('@/features/countries/CountryForm'), 'CountryEditPage') },
  { title: 'New webhook', path: '/webhooks/new', element: lazyPage(() => import('@/features/webhooks/WebhookCreate'), 'WebhookCreate'), permission: 'webhooks.manage' },
  { title: 'Webhook detail', path: '/webhooks/:id', element: lazyPage(() => import('@/features/webhooks/WebhookDetail'), 'WebhookDetail') },
  { title: 'New blog post', path: '/blog/posts/new', element: lazyPage(() => import('@/features/blog/BlogPostCreate'), 'BlogPostCreate') },
  { title: 'Blog post detail', path: '/blog/posts/:id', element: lazyPage(() => import('@/features/blog/BlogPostDetail'), 'BlogPostDetail') },
  { title: 'New tax rule', path: '/tax/new', element: lazyPage(() => import('@/features/tax/TaxRuleForm'), 'TaxRuleCreatePage'), permission: 'settings.manage' },
  { title: 'Edit tax rule', path: '/tax/:id', element: lazyPage(() => import('@/features/tax/TaxRuleForm'), 'TaxRuleEditPage'), permission: 'settings.manage' },
  { title: 'New shipping zone', path: '/shipping/new', element: lazyPage(() => import('@/features/shipping/ShippingZonesList'), 'ShippingZonesList'), permission: 'settings.manage' },
  { title: 'Edit shipping zone', path: '/shipping/:id', element: lazyPage(() => import('@/features/shipping/ShippingZonesList'), 'ShippingZonesList'), permission: 'settings.manage' },
  { title: 'Segment detail', path: '/segments/:id', element: lazyPage(() => import('@/features/segments/SegmentDetailPage'), 'SegmentDetailPage'), permission: 'promotions.view' },
  { title: 'Company detail', path: '/b2b/companies/:id', element: lazyPage(() => import('@/features/b2b/CompanyDetailPage'), 'CompanyDetailPage'), permission: 'b2b.view', feature: 'b2b' },
  {
    title: 'Promotion analytics redirect',
    path: '/promotions/analytics',
    element: <Navigate to="/promotions" replace />,
    permission: 'promotions.view',
  },
  { title: 'New promotion', path: '/promotions/new', element: lazyPage(() => import('@/features/promotions/PromotionPage'), 'PromotionPage'), permission: 'promotions.manage' },
  { title: 'Edit promotion', path: '/promotions/:id', element: lazyPage(() => import('@/features/promotions/PromotionPage'), 'PromotionPage'), permission: 'promotions.manage' },
  { title: 'New page', path: '/cms/pages/new', element: lazyPage(() => import('@/features/cms/CmsPageCreate'), 'CmsPageCreate') },
  { title: 'CMS page detail', path: '/cms/pages/:id', element: lazyPage(() => import('@/features/cms/CmsPageDetail'), 'CmsPageDetail') },
  {
    title: 'Reusable blocks redirect',
    path: '/settings/email/blocks',
    element: <Navigate to="/emails?tab=blocks" replace />,
  },
  // Transactional email is ordinary library emails delivered by Pulse flows, so these
  // standalone template paths redirect to the unified Emails hub
  { title: 'Email templates redirect', path: '/settings/email', element: <Navigate to="/emails" replace /> },
  { title: 'Email template redirect', path: '/settings/email/:key', element: <Navigate to="/emails" replace /> },
  // The settings sidebar selection rides the URL - one route with an optional group segment, not
  // two, since the shared route object re-renders the page between panes instead of remounting it
  // (the static /settings/email* redirects above still outrank it in React Router's ranking)
  {
    title: 'Settings section',
    path: '/settings/:sectionSlug/:groupSlug?',
    element: lazyPage(() => import('@/features/settings/SettingsPage'), 'SettingsPage'),
    permission: 'config.view',
  },
  // Pulse list pages - reached from the /pulse overview (and in-app links), not
  // the sidebar, so they live here as route-only entries rather than nav items
  // The unified Emails hub carries three tabs (?tab=emails|templates|blocks)
  // Registered as a Marketing nav leaf (feature '!signals') instead of a
  // detail route: normally reached via the Pulse hub, but with Signals off
  // the sidebar entry keeps transactional email templates editable
  { title: 'Email blocks', path: '/emails/blocks', element: <Navigate to="/emails?tab=blocks" replace /> },
  { title: 'Campaigns', path: '/campaigns', element: lazyPage(() => import('@/features/campaigns/CampaignsList'), 'CampaignsList'), permission: 'campaigns.view' },
  { title: 'Flows', path: '/flows', element: lazyPage(() => import('@/features/flows/FlowsList'), 'FlowsList'), permission: 'flows.view' },
  { title: 'Signup forms', path: '/forms', element: lazyPage(() => import('@/features/forms/FormsList'), 'FormsList'), permission: 'forms.view' },
  { title: 'New form', path: '/forms/new', element: lazyPage(() => import('@/features/forms/FormBuilder'), 'FormBuilder'), permission: 'forms.manage' },
  { title: 'Edit form', path: '/forms/:id', element: lazyPage(() => import('@/features/forms/FormBuilder'), 'FormBuilder'), permission: 'forms.view' },
  { title: 'Suppression list', path: '/emails/suppressions', element: lazyPage(() => import('@/features/deliverability/SuppressionList'), 'SuppressionList'), permission: 'emails.view' },
  // Lens heatmap viewer - reached from the /lens overview's top-pages list
  // (page + viewport carried in the query string, since page keys contain '/')
  { title: 'Heatmap', path: '/lens/heatmap', element: lazyPage(() => import('@/features/lens/LensPageDetail'), 'LensPageDetail'), permission: 'lens.view' },
  // Lens bot / crawler analytics - reached from the /lens overview header
  { title: 'Bot traffic', path: '/lens/bots', element: lazyPage(() => import('@/features/lens/LensBots'), 'LensBots'), permission: 'lens.view' },
  // Lens session replay (rrweb): the channel recordings browser + the player (reached from /lens, the recordings list,
  // and the Synapse profiles) - Static /lens/sessions outranks the /:id param in React Router ranking
  { title: 'Recordings', path: '/lens/sessions', element: lazyPage(() => import('@/features/lens/LensRecordings'), 'LensRecordings'), permission: 'lens.view' },
  { title: 'Session replay', path: '/lens/sessions/:id', element: lazyPage(() => import('@/features/lens/LensSession'), 'LensSession'), permission: 'lens.view' },
  // Lens UX signals - JS errors + Core Web Vitals + on-site search (reached from
  // the /lens "Explore" cards)
  { title: 'JS errors', path: '/lens/errors', element: lazyPage(() => import('@/features/lens/LensErrors'), 'LensErrors'), permission: 'lens.view' },
  { title: 'Performance', path: '/lens/performance', element: lazyPage(() => import('@/features/lens/LensPerformance'), 'LensPerformance'), permission: 'lens.view' },
  { title: 'Search analytics', path: '/lens/search', element: lazyPage(() => import('@/features/lens/LensSearch'), 'LensSearch'), permission: 'lens.view' },
  // Admin inbox - reached from the header bell's "View all"
  { title: 'Inbox', path: '/inbox', element: lazyPage(() => import('@/features/notifications/InboxPage'), 'InboxPage'), permission: 'notifications.view' },
  // CMS list pages - route-only; reached from the /cms hub cards
  { title: 'Pages', path: '/cms/pages', element: lazyPage(() => import('@/features/cms/CmsPagesPage'), 'CmsPagesPage'), permission: 'cms.view' },
  { title: 'Page types', path: '/cms/page-types', element: lazyPage(() => import('@/features/cms/CmsPageTypesPage'), 'CmsPageTypesPage'), permission: 'cms.view' },
  { title: 'New page type', path: '/cms/page-types/new', element: lazyPage(() => import('@/features/cms/CmsPageTypeCreate'), 'CmsPageTypeCreate'), permission: 'cms.view' },
  { title: 'Page type detail', path: '/cms/page-types/:id', element: lazyPage(() => import('@/features/cms/CmsPageTypeDetail'), 'CmsPageTypeDetail'), permission: 'cms.view' },
  { title: 'Block types', path: '/cms/block-types', element: lazyPage(() => import('@/features/cms/CmsBlockTypesPage'), 'CmsBlockTypesPage'), permission: 'cms.view' },
  { title: 'New block type', path: '/cms/block-types/new', element: lazyPage(() => import('@/features/cms/CmsBlockTypeDetail'), 'CmsBlockTypeCreate'), permission: 'cms.view' },
  { title: 'Block type detail', path: '/cms/block-types/:id', element: lazyPage(() => import('@/features/cms/CmsBlockTypeDetail'), 'CmsBlockTypeDetail'), permission: 'cms.view' },
  { title: 'Page attributes', path: '/cms/attributes', element: lazyPage(() => import('@/features/cms/CmsAttributesPage'), 'CmsAttributesPage'), permission: 'cms.view' },
  { title: 'New attribute', path: '/cms/attributes/new', element: lazyPage(() => import('@/features/cms/CmsAttributeCreate'), 'CmsAttributeCreate'), permission: 'cms.view' },
  { title: 'Page attribute detail', path: '/cms/attributes/:id', element: lazyPage(() => import('@/features/cms/CmsAttributeDetail'), 'CmsAttributeDetail'), permission: 'cms.view' },
  { title: 'Sections', path: '/cms/sections', element: lazyPage(() => import('@/features/cms/CmsSnippetsPage'), 'CmsSnippetsPage'), permission: 'cms.view' },
  { title: 'Tags', path: '/cms/tags', element: lazyPage(() => import('@/features/cms/CmsTagsPage'), 'CmsTagsPage'), permission: 'cms.view' },
  { title: 'Forms', path: '/cms/forms', element: lazyPage(() => import('@/features/webforms/WebFormsList'), 'WebFormsList'), permission: 'webforms.view' },
  { title: 'New form', path: '/cms/forms/new', element: lazyPage(() => import('@/features/webforms/WebFormDetail'), 'WebFormDetail'), permission: 'webforms.manage' },
  { title: 'Form detail', path: '/cms/forms/:id', element: lazyPage(() => import('@/features/webforms/WebFormDetail'), 'WebFormDetail'), permission: 'webforms.view' },
  { title: 'Form submissions', path: '/cms/forms/:id/submissions', element: lazyPage(() => import('@/features/webforms/WebFormSubmissions'), 'WebFormSubmissions'), permission: 'webforms.submissions_view' },
  { title: 'Menus', path: '/menus', element: lazyPage(() => import('@/features/menus/MenusList'), 'MenusList'), permission: 'cms.view' },
  { title: 'Posts', path: '/blog/posts', element: lazyPage(() => import('@/features/blog/BlogPostsList'), 'BlogPostsList'), permission: 'blog.view' },
  { title: 'Blog taxonomy', path: '/blog/taxonomy', element: lazyPage(() => import('@/features/blog/BlogTaxonomyPage'), 'BlogTaxonomyPage'), permission: 'blog.view' },
  { title: 'New category', path: '/blog/categories/new', element: lazyPage(() => import('@/features/blog/BlogCategoryEditor'), 'BlogCategoryEditor'), permission: 'blog.view' },
  { title: 'Category detail', path: '/blog/categories/:id', element: lazyPage(() => import('@/features/blog/BlogCategoryEditor'), 'BlogCategoryEditor'), permission: 'blog.view' },
  { title: 'New tag', path: '/blog/tags/new', element: lazyPage(() => import('@/features/blog/BlogTagEditor'), 'BlogTagEditor'), permission: 'blog.view' },
  { title: 'Tag detail', path: '/blog/tags/:id', element: lazyPage(() => import('@/features/blog/BlogTagEditor'), 'BlogTagEditor'), permission: 'blog.view' },
  { title: 'New author', path: '/blog/authors/new', element: lazyPage(() => import('@/features/blog/BlogAuthorEditor'), 'BlogAuthorEditor'), permission: 'blog.view' },
  { title: 'Author detail', path: '/blog/authors/:id', element: lazyPage(() => import('@/features/blog/BlogAuthorEditor'), 'BlogAuthorEditor'), permission: 'blog.view' },
  // Prism experiments list + editor - reached from the /prism overview hub
  // (route-only) - Static paths outrank the /:id param in React Router ranking
  { title: 'Experiments', path: '/prism/experiments', element: lazyPage(() => import('@/features/prism/PrismList'), 'PrismList'), permission: 'experiments.view' },
  { title: 'New experiment', path: '/prism/new', element: lazyPage(() => import('@/features/prism/PrismEditor'), 'PrismEditor'), permission: 'experiments.manage' },
  { title: 'Experiment', path: '/prism/:id', element: lazyPage(() => import('@/features/prism/PrismEditor'), 'PrismEditor'), permission: 'experiments.view' },
  // Synapse per-customer behavioural profile - reached from the /synapse overview's "most active customers" list (and
  // in-app links), not the sidebar Lives under Synapse, not the operational customer record
  { title: 'Customer profile', path: '/synapse/customers/:id', element: lazyPage(() => import('@/features/synapse/SynapseCustomer'), 'SynapseCustomer'), permission: 'customers.view' },
  { title: 'Visitor profile', path: '/synapse/visitors/:anonId', element: lazyPage(() => import('@/features/synapse/SynapseVisitor'), 'SynapseVisitor'), permission: 'customers.view' },
  { title: 'Funnel', path: '/synapse/funnels', element: lazyPage(() => import('@/features/synapse/SynapseFunnel'), 'SynapseFunnel'), permission: 'customers.view' },
  { title: 'Retention', path: '/synapse/retention', element: lazyPage(() => import('@/features/synapse/SynapseRetention'), 'SynapseRetention'), permission: 'customers.view' },
  { title: 'Segments', path: '/synapse/segments', element: lazyPage(() => import('@/features/synapse/SynapseSegments'), 'SynapseSegments'), permission: 'customers.view' },
  { title: 'Segment', path: '/synapse/segments/:id', element: lazyPage(() => import('@/features/synapse/SynapseSegment'), 'SynapseSegment'), permission: 'customers.view' },
  { title: 'Flow', path: '/synapse/flows', element: lazyPage(() => import('@/features/synapse/SynapseFlow'), 'SynapseFlow'), permission: 'customers.view' },
  { title: 'Engagement', path: '/synapse/stickiness', element: lazyPage(() => import('@/features/synapse/SynapseStickiness'), 'SynapseStickiness'), permission: 'customers.view' },
  { title: 'Insights', path: '/synapse/insights', element: lazyPage(() => import('@/features/synapse/SynapseInsights'), 'SynapseInsights'), permission: 'customers.view' },
  { title: 'New block', path: '/emails/blocks/new', element: lazyPage(() => import('@/features/blocks/BlockLibraryDetail'), 'BlockLibraryDetail'), permission: 'emails.manage' },
  { title: 'Block detail', path: '/emails/blocks/:slug', element: lazyPage(() => import('@/features/blocks/BlockLibraryDetail'), 'BlockLibraryDetail'), permission: 'emails.view' },
  { title: 'New campaign', path: '/campaigns/new', element: lazyPage(() => import('@/features/campaigns/CampaignComposer'), 'CampaignComposer'), permission: 'campaigns.manage' },
  { title: 'Campaign detail', path: '/campaigns/:id', element: lazyPage(() => import('@/features/campaigns/CampaignComposer'), 'CampaignComposer'), permission: 'campaigns.view' },
  { title: 'New flow', path: '/flows/new', element: lazyPage(() => import('@/features/flows/FlowBuilder'), 'FlowBuilder'), permission: 'flows.manage' },
  { title: 'Flow detail', path: '/flows/:id', element: lazyPage(() => import('@/features/flows/FlowBuilder'), 'FlowBuilder'), permission: 'flows.view' },
  { title: 'New email', path: '/emails/new', element: lazyPage(() => import('@/features/emails/EmailLibraryDetail'), 'EmailLibraryDetail'), permission: 'emails.manage' },
  { title: 'Email detail', path: '/emails/:id', element: lazyPage(() => import('@/features/emails/EmailLibraryDetail'), 'EmailLibraryDetail'), permission: 'emails.view' },
  { title: 'New menu', path: '/menus/new', element: lazyPage(() => import('@/features/menus/MenusList'), 'MenuCreatePane') },
  { title: 'Menu detail', path: '/menus/:id', element: lazyPage(() => import('@/features/menus/MenuDetail'), 'MenuDetail') },
  {
    title: 'URL rewrites redirect',
    path: '/catalog/url-rewrites',
    element: <Navigate to="/seo/url-rewrites" replace />,
  },
  // Easter egg - deliberately route-only and permissionless; the 404 page knows the way in
  { title: 'Preferences', path: '/preferences', element: lazyPage(() => import('@/features/preferences/PreferencesPage'), 'PreferencesPage') },
  { title: 'Boneyard', path: '/bones', element: lazyPage(() => import('@/features/boneyard/BoneyardPage'), 'BoneyardPage') },
]
for (const leaf of detailLeaves) registerDetailRoute(leaf)
