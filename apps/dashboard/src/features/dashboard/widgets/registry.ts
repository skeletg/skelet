import type { ComponentType } from 'react'
import { AbandonedCartsWidget } from './abandoned-carts'
import { AccountWidget } from './account'
import { AuditWidget } from './audit'
import { EmailWidget } from './email'
import { ExperimentsWidget } from './experiments'
import { FulfillmentWidget } from './fulfillment'
import { InboxWidget } from './inbox'
import { JobsWidget } from './jobs'
import { LowStockWidget } from './low-stock'
import { ModerationWidget } from './moderation'
import { OrderChartsWidget } from './order-charts'
import { PluginWidget } from './plugin-slot'
import { PromotionsWidget } from './promotions'
import { QuickActionsWidget } from './quick-actions'
import { RecentCustomersWidget } from './recent-customers'
import { RecentOrdersWidget } from './recent-orders'
import { RevenueWidget } from './revenue'
import { StatsWidget } from './stats'
import { SystemHealthWidget } from './system-health'
import { TopProductsWidget } from './top-products'
import { TrafficWidget } from './traffic'

// Every section the dashboard can show
//
// A section owns its own query and Home only mounts the ones being shown, so a section that is off
// costs nothing - which is what makes a list this long affordable. `defaultHidden` is the library
// half: those are switched on from Home's Customize mode or from Preferences, and the ones without
// it are the page you get on a fresh account
export const HOME_LAYOUT_PREF_KEY = 'home:layout'

export type HomeWidget = {
  id: string // storage key: renaming one drops it from every admin's saved arrangement
  title: string // names the section in the arrange lists, not on the page - each section titles itself
  permission?: string
  feature?: string
  defaultHidden?: boolean
  Component: ComponentType
}

export const HOME_WIDGETS: HomeWidget[] = [
  { id: 'quick-actions', title: 'Quick actions', Component: QuickActionsWidget },
  { id: 'stats', title: 'At a glance', Component: StatsWidget },
  { id: 'fulfillment', title: 'Order queues', permission: 'orders.view', Component: FulfillmentWidget },
  { id: 'revenue', title: 'Revenue', permission: 'reports.view', Component: RevenueWidget },
  { id: 'moderation', title: 'Waiting on you', Component: ModerationWidget },
  { id: 'low-stock', title: 'Low stock', permission: 'inventory.view', Component: LowStockWidget },
  { id: 'inbox', title: 'Inbox', permission: 'notifications.view', Component: InboxWidget },
  { id: 'order-charts', title: 'Orders', permission: 'reports.view', Component: OrderChartsWidget },
  { id: 'recent-orders', title: 'Recent orders', permission: 'orders.view', Component: RecentOrdersWidget },
  { id: 'recent-customers', title: 'Recent customers', permission: 'customers.view', Component: RecentCustomersWidget },
  { id: 'account', title: 'Your account', Component: AccountWidget },

  // the library
  { id: 'top-products', title: 'Top products', permission: 'reports.view', defaultHidden: true, Component: TopProductsWidget },
  { id: 'abandoned-carts', title: 'Abandoned carts', permission: 'orders.view', defaultHidden: true, Component: AbandonedCartsWidget },
  { id: 'promotions', title: 'Promotions', permission: 'promotions.view', defaultHidden: true, Component: PromotionsWidget },
  { id: 'traffic', title: 'Traffic', permission: 'lens.view', feature: 'signals', defaultHidden: true, Component: TrafficWidget },
  { id: 'email', title: 'Email engagement', permission: 'emails.view', feature: 'signals', defaultHidden: true, Component: EmailWidget },
  { id: 'experiments', title: 'Experiments', permission: 'experiments.view', feature: 'signals', defaultHidden: true, Component: ExperimentsWidget },
  { id: 'jobs', title: 'Scheduled jobs', permission: 'scheduler.view', defaultHidden: true, Component: JobsWidget },
  { id: 'system-health', title: 'System health', permission: 'settings.manage', defaultHidden: true, Component: SystemHealthWidget },
  { id: 'audit', title: 'Recent admin activity', permission: 'audit.view', defaultHidden: true, Component: AuditWidget },

  // last on purpose - a plugin's section belongs after the core page, not in the middle of it
  { id: 'plugins', title: 'Plugin widgets', Component: PluginWidget },
]

// Pure so it can be tested without mounting the page - a section whose permission the admin doesn't hold, or whose feature switch is off,
// is absent rather than empty. Gates read as unmet while the grants are still loading, so nothing flashes in and out
export function visibleWidgets(
  widgets: HomeWidget[],
  gates: {
    has: (slug: string) => boolean
    featureEnabled: (flag: string) => boolean
    permsLoading?: boolean
    featuresLoading?: boolean
  },
): HomeWidget[] {
  return widgets.filter((w) => {
    if (w.permission && (gates.permsLoading || !gates.has(w.permission))) return false
    if (w.feature && (gates.featuresLoading || !gates.featureEnabled(w.feature))) return false
    return true
  })
}
