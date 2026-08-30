import { Link } from 'react-router-dom'
import { usePreference } from '@/lib/admin-prefs'
import { EMPTY_LAYOUT, applyLayout, isHiddenInLayout, type OrderedLayout } from '@/lib/layout-pref'
import { usePermissions } from '@/shared/auth/usePermissions'
import { QUICK_ACTIONS, QUICK_ACTIONS_PREF_KEY } from '@/features/dashboard/home-items'
import { WidgetSection } from './primitives'

export function QuickActionsWidget() {
  const { has, loading } = usePermissions()
  const [layout] = usePreference<OrderedLayout>(QUICK_ACTIONS_PREF_KEY, EMPTY_LAYOUT)
  const actions = applyLayout(
    QUICK_ACTIONS.filter((a) => !a.permission || has(a.permission)),
    layout,
  ).filter((a) => !isHiddenInLayout(layout, a.id))

  return (
    <WidgetSection
      title="Quick actions"
      description="Jump into the most common admin tasks."
      loading={loading}
      empty={actions.length === 0}
    >
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {actions.map((a) => (
          <Link
            key={a.path}
            to={a.path}
            className="group flex items-start gap-3 rounded-lg border p-4 transition hover:border-primary/40 hover:bg-muted/40"
          >
            <span className="grid size-9 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
              <a.icon className="size-5" />
            </span>
            <span className="min-w-0">
              <span className="block font-medium group-hover:text-primary">{a.title}</span>
              <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">
                {a.description}
              </span>
            </span>
          </Link>
        ))}
      </div>
    </WidgetSection>
  )
}
