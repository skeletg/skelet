// What a dashboard plugin may import, as `@skelet/dashboard-sdk`. The dashboard's build points that name here, so a
// plugin from npm registers into the same registries and draws with the same components the dashboard's own pages do;
// the package published under it holds the types alone. Anything not exported here is the dashboard's own to change
//
// A plugin also takes react and @apollo/client as peers, and queries through the dashboard's Apollo client from inside
// its components with @apollo/client/react's hooks

// Extension points
export {
  registerDetailSection,
  type DetailSectionEntry,
  type DetailSectionMode,
  type DetailSectionPlacement,
} from '@/plugins/detail-section-registry'
export { registerDetailField, type DetailFieldEntry, type DetailFieldType } from '@/plugins/field-registry'
export { registerSlot, type SlotEntry } from '@/plugins/slot-registry'
export { registerHomeWidget, type PluginHomeWidget } from '@/plugins/home-widget-registry'
export {
  registerListColumn,
  registerListFilter,
  registerListRowAction,
  registerListMassAction,
  metadataColumn,
  dateColumn,
  moneyColumn,
  badgeColumn,
  linkColumn,
  type ListColumnEntry,
  type ListFilterEntry,
  type ListRowActionEntry,
  type ListMassActionEntry,
  type RowAction,
  type BulkAction,
  type DataTableFilter,
} from '@/components/data-table'
export { registerNavNode, registerFooterNode, registerDetailRoute, extendGroup } from '@/nav/registry'
export type { NavLeaf, NavGroup, NavNode, NavSubgroup } from '@/nav/types'
export { registerNavLabels } from '@/nav/nav-i18n'
export { lazyPage } from '@/nav/lazy'
export { registerLexicalBlock, type LexicalBlockDefinition } from '@/features/catalog/lexical-blocks/registry'

// A plugin's own pages and sections, built the way core's are
export { DetailPageLayout, DetailPageHeader, DetailPageBody, DetailPageSection } from '@/components/layout/detail-page'
export { DataTable } from '@/components/data-table'
export { useSaveSection, type SectionRegistration, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
export { usePermissions } from '@/shared/auth/usePermissions'
export { cn } from '@/lib/utils'
export { formatCents, formatDate, formatDateTime, formatNumber, timeAgo } from '@/lib/format'

// The UI kit
export * from '@/components/ui/alert'
export * from '@/components/ui/badge'
export * from '@/components/ui/button'
export * from '@/components/ui/checkbox'
export * from '@/components/ui/dialog'
export * from '@/components/ui/dropdown-menu'
export * from '@/components/ui/input'
export * from '@/components/ui/label'
export * from '@/components/ui/loading-state'
export * from '@/components/ui/select'
export * from '@/components/ui/skeleton'
export * from '@/components/ui/switch'
export * from '@/components/ui/table'
export * from '@/components/ui/tabs'
export * from '@/components/ui/textarea'
export * from '@/components/ui/tooltip'
