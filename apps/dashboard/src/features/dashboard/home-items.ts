import {
  GlobeIcon,
  PackagePlusIcon,
  ShapesIcon,
  ShoppingCartIcon,
  TagsIcon,
  UsersIcon,
  type LucideIcon,
} from 'lucide-react'

// The Home page's two arrangeable lists, declared apart from the page itself so Preferences can offer pickers for them without pulling in
// the whole dashboard chunk. Ids are storage keys - once an admin has arranged a list, renaming one drops that entry from their layout

export const QUICK_ACTIONS_PREF_KEY = 'home:quickActions'
export const TILES_PREF_KEY = 'home:tiles'

export type QuickAction = {
  id: string
  title: string
  description: string
  path: string
  permission?: string
  icon: LucideIcon
}

export const QUICK_ACTIONS: QuickAction[] = [
  { id: 'new-product', title: 'New product', description: 'Create a catalog product and its first translation.', path: '/products/new', permission: 'catalog.manage', icon: PackagePlusIcon },
  { id: 'new-collection', title: 'New collection', description: 'Build a curated product collection.', path: '/collections/new', permission: 'catalog.manage', icon: ShapesIcon },
  { id: 'orders', title: 'Orders', description: 'Review incoming and in-progress orders.', path: '/orders', permission: 'orders.view', icon: ShoppingCartIcon },
  { id: 'customers', title: 'Customers', description: 'Look up customers and groups.', path: '/customers', permission: 'customers.view', icon: UsersIcon },
  { id: 'attributes', title: 'Attributes', description: 'Adjust product attributes and options.', path: '/catalog/attributes', permission: 'catalog.view', icon: TagsIcon },
  { id: 'url-rewrites', title: 'URL rewrites', description: 'Manage SEO paths and redirects from old URLs.', path: '/seo/url-rewrites', permission: 'catalog.view', icon: GlobeIcon },
]

// Labels live here rather than in the tile's own copy, so the picker names a tile the same way the
// page does - the values themselves are computed in the stats section, next to the data
//
// The money tiles are gated on reports.view because that is what the salesReport query behind them
// enforces; an admin with orders but not reports keeps the counts and loses the figures
export type TileDef = { id: string; label: string; permission: string; defaultHidden?: boolean }

export const TILE_DEFS: TileDef[] = [
  { id: 'revenue', label: 'Revenue', permission: 'reports.view' },
  { id: 'orders', label: 'Orders', permission: 'reports.view' },
  { id: 'aov', label: 'Avg order value', permission: 'reports.view' },
  { id: 'pending', label: 'Pending', permission: 'orders.view' },
  { id: 'customers', label: 'Customers', permission: 'customers.view' },
  { id: 'products', label: 'Products', permission: 'catalog.view' },
  { id: 'collections', label: 'Collections', permission: 'catalog.view' },
  // reporting-grade extras, off until asked for - a dashboard opening with ten tiles reads as noise
  { id: 'margin', label: 'Margin', permission: 'reports.view', defaultHidden: true },
  { id: 'refunds', label: 'Refunds', permission: 'reports.view', defaultHidden: true },
  { id: 'ordering-customers', label: 'Customers who ordered', permission: 'reports.view', defaultHidden: true },
]
