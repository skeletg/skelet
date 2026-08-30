import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { DETAIL_TOC_PREF_KEY, DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { usePreference } from '@/lib/admin-prefs'
import { useHiddenGroups } from '@/lib/nav-hidden'
import { STICKY_TOOLBAR_PREF_KEY } from '@/components/data-table/DataTableToolbar'
import { getNavNodes } from '@/nav/registry'
import { groupItems, isGroup } from '@/nav/types'
import { useVisibleNav } from '@/nav/use-visible-nav'
import {
  QUICK_ACTIONS,
  QUICK_ACTIONS_PREF_KEY,
  TILES_PREF_KEY,
  TILE_DEFS,
} from '@/features/dashboard/home-items'
import { HOME_LAYOUT_PREF_KEY, HOME_WIDGETS } from '@/features/dashboard/widgets/registry'
import { useFeatures } from '@/shared/features/useFeatures'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ArrangeList } from './ArrangeList'

// Sidebar groups, hide-only - reordering the nav would fight the muscle memory the pin feature
// already serves, and a hidden group's pages stay reachable through ⌘K, a pin or a direct link
function SidebarGroups() {
  const { nodes } = useVisibleNav(getNavNodes())
  const { hidden, isHidden, toggle, showAll } = useHiddenGroups()
  const groups = nodes.filter(isGroup).filter((g) => g.id !== undefined)

  return (
    // capped like ArrangeList, so the switches don't sit under the floating section index
    <div className="max-w-3xl space-y-2">
      <div className="divide-y rounded-md border">
        {groups.map((group) => {
          const off = isHidden(group.id ?? '')
          const count = groupItems(group).length
          return (
            <label
              key={group.id}
              className="flex cursor-pointer items-center gap-3 px-3 py-2"
            >
              <span
                className={cn('h-8 w-0.5 shrink-0 rounded-full', off ? 'bg-transparent' : 'bg-primary/40')}
              />
              <span className="min-w-0 flex-1">
                <span className={cn('block text-sm', off && 'text-muted-foreground')}>{group.title}</span>
                <span className="block text-xs text-muted-foreground">
                  {count} {count === 1 ? 'page' : 'pages'}
                </span>
              </span>
              <Switch checked={!off} onCheckedChange={() => toggle(group.id ?? '')} />
            </label>
          )
        })}
      </div>
      {hidden.length > 0 && (
        <Button variant="ghost" size="sm" onClick={showAll}>
          Show all groups
        </Button>
      )}
    </div>
  )
}

function InterfaceToggles() {
  const [sticky, setSticky] = usePreference<boolean>(STICKY_TOOLBAR_PREF_KEY, false)
  const [toc, setToc] = usePreference<boolean>(DETAIL_TOC_PREF_KEY, true)
  // storing a default just deletes the key, so an untouched account keeps no row
  const rows = [
    {
      id: 'toc',
      label: 'Section index on detail pages',
      hint: 'The floating list of sections down the right edge of long pages.',
      on: toc,
      set: (v: boolean) => setToc(v ? undefined : false),
    },
    {
      id: 'sticky',
      label: 'Follow list filters while scrolling',
      hint: 'The table toolbar detaches into a floating island once rows scroll past it.',
      on: sticky,
      set: (v: boolean) => setSticky(v ? true : undefined),
    },
  ]

  return (
    <div className="max-w-3xl divide-y rounded-md border">
      {rows.map((row) => (
        <div key={row.id} className="flex items-center gap-3 px-3 py-2">
          <span className="min-w-0 flex-1">
            <Label htmlFor={`iface-${row.id}`} className="block text-sm font-normal">
              {row.label}
            </Label>
            <span className="block text-xs text-muted-foreground">{row.hint}</span>
          </span>
          <Switch id={`iface-${row.id}`} checked={row.on} onCheckedChange={row.set} />
        </div>
      ))}
    </div>
  )
}

export function ShellSection() {
  const { has } = usePermissions()
  const { enabled: featureEnabled } = useFeatures()
  const allowed = (permission?: string) => !permission || has(permission) // has() already passes superusers

  return (
    <>
      <DetailPageSection
        title="Sidebar groups"
        description="Turn off the parts of the store you don't work in. Their pages stay reachable through search, a pin or a link."
      >
        <SidebarGroups />
      </DetailPageSection>

      <DetailPageSection
        title="Interface"
        description="Two bits of chrome that are a matter of taste."
      >
        <InterfaceToggles />
      </DetailPageSection>

      <DetailPageSection
        title="Dashboard sections"
        description="Everything your dashboard can show. The ones switched off are a library - turn on what you need, in the order you want it."
      >
        <ArrangeList
          prefKey={HOME_LAYOUT_PREF_KEY}
          items={HOME_WIDGETS.filter(
            (w) => allowed(w.permission) && (!w.feature || featureEnabled(w.feature)),
          ).map((w) => ({ id: w.id, label: w.title, defaultHidden: w.defaultHidden }))}
        />
      </DetailPageSection>

      <DetailPageSection
        title="Dashboard quick actions"
        description="The shortcut tiles at the top of your dashboard, in the order you want them."
      >
        <ArrangeList
          prefKey={QUICK_ACTIONS_PREF_KEY}
          items={QUICK_ACTIONS.filter((a) => allowed(a.permission)).map((a) => ({
            id: a.id,
            label: a.title,
            hint: a.description,
          }))}
        />
      </DetailPageSection>

      <DetailPageSection
        title="Dashboard figures"
        description="The at-a-glance numbers. Hide the ones that don't mean anything for your store."
      >
        <ArrangeList
          prefKey={TILES_PREF_KEY}
          items={TILE_DEFS.filter((t) => allowed(t.permission)).map((t) => ({
            id: t.id,
            label: t.label,
            defaultHidden: t.defaultHidden,
          }))}
        />
      </DetailPageSection>
    </>
  )
}
