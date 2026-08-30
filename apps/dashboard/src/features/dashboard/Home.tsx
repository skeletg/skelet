import { Fragment, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, EyeIcon, EyeOffIcon, SlidersHorizontalIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DetailPageBody, DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { usePreference } from '@/lib/admin-prefs'
import {
  EMPTY_LAYOUT,
  applyLayout,
  isHiddenInLayout,
  moveInLayout,
  toggleInLayout,
  type OrderedLayout,
} from '@/lib/layout-pref'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'
import { MeDocument } from '@/shared/auth/Me.gql'
import {
  HOME_LAYOUT_PREF_KEY,
  HOME_WIDGETS,
  visibleWidgets,
} from '@/features/dashboard/widgets/registry'

// The dashboard is a stack of sections the admin arranges. This file owns the shell and the arrangement only - every section lives in its
// own file under widgets/ and fetches its own data, so a section that is switched off is never mounted and costs nothing
export function Home() {
  const { data } = useQuery(MeDocument, { fetchPolicy: 'cache-first' })
  const me = data?.me
  const { has, loading: permsLoading } = usePermissions()
  const { enabled: featureEnabled, loading: featuresLoading } = useFeatures()
  const [layout, setLayout] = usePreference<OrderedLayout>(HOME_LAYOUT_PREF_KEY, EMPTY_LAYOUT)
  const [customizing, setCustomizing] = useState(false)

  // the same gating rules the sidebar uses - hidden while grants load rather than flashing in
  const available = visibleWidgets(HOME_WIDGETS, {
    has,
    featureEnabled,
    permsLoading,
    featuresLoading,
  })
  const ordered = applyLayout(available, layout)
  const ids = ordered.map((w) => w.id)

  const greeting = me?.firstName ? `Welcome back, ${me.firstName}` : 'Welcome back'

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Dashboard"
        badges={me?.isSuperuser ? <Badge variant="secondary">Superuser</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            {customizing
              ? 'Reorder sections and switch on the ones you want - your arrangement follows you to any device.'
              : `${greeting} - here's what's happening across your store.`}
          </span>
        }
        actions={
          customizing ? (
            <>
              <Button variant="ghost" size="sm" onClick={() => setLayout(undefined)}>
                Reset
              </Button>
              <Button size="sm" onClick={() => setCustomizing(false)}>
                Done
              </Button>
            </>
          ) : (
            <Button variant="outline" size="sm" onClick={() => setCustomizing(true)}>
              <SlidersHorizontalIcon className="size-4" />
              Customize
            </Button>
          )
        }
      />
      <DetailPageBody>
        {ordered.map((w, i) => {
          const hidden = isHiddenInLayout(layout, w.id, w.defaultHidden)
          if (!customizing) {
            return hidden ? null : (
              <Fragment key={w.id}>
                <w.Component />
              </Fragment>
            )
          }
          return (
            <div key={w.id}>
              <div className="flex items-center gap-1 bg-muted/40 px-6 py-1.5">
                <span className="text-xs font-medium">{w.title}</span>
                {hidden && <span className="text-xs text-muted-foreground">· hidden</span>}
                <div className="ml-auto flex items-center gap-0.5">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6"
                    disabled={i === 0}
                    onClick={() => setLayout((prev) => moveInLayout(prev, ids, w.id, -1))}
                    aria-label={`Move ${w.title} up`}
                  >
                    <ArrowUpIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6"
                    disabled={i === ordered.length - 1}
                    onClick={() => setLayout((prev) => moveInLayout(prev, ids, w.id, 1))}
                    aria-label={`Move ${w.title} down`}
                  >
                    <ArrowDownIcon className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6"
                    onClick={() => setLayout((prev) => toggleInLayout(prev, w.id, w.defaultHidden))}
                    aria-label={hidden ? `Show ${w.title}` : `Hide ${w.title}`}
                  >
                    {hidden ? <EyeOffIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
                  </Button>
                </div>
              </div>
              {/* a hidden section stays unmounted while browsing the library too, so opening
                  Customize doesn't fire every query it contains */}
              {!hidden && <w.Component />}
            </div>
          )
        })}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
