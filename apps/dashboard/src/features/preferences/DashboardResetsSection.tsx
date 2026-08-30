import { Trash2Icon } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import {
  usePreference,
  usePreferenceKeys,
  usePrefsLoaded,
  useRemovePreferences,
} from '@/lib/admin-prefs'
import { usePinnedPaths } from '@/lib/nav-pins'
import type { ViewsPref } from '@/components/data-table/SavedViews'

const EMPTY_VIEWS: ViewsPref = { views: [] }
const EPHEMERAL = 'table:__ephemeral' // keyless tables park here, it is not a real layout

// Whatever the admin has customized elsewhere, undoable from one place - a saved view could
// otherwise only be deleted from the list that made it
export function DashboardResetsSection() {
  const loaded = usePrefsLoaded()
  const viewKeys = usePreferenceKeys('views:')
  const tableKeys = usePreferenceKeys('table:').filter((k) => k !== EPHEMERAL)
  const homeKeys = usePreferenceKeys('home:layout')
  const remove = useRemovePreferences()
  const { pins } = usePinnedPaths()

  if (!loaded) {
    return (
      <DetailPageSection title="Your customizations">
        <p className="text-sm text-muted-foreground">Loading your preferences…</p>
      </DetailPageSection>
    )
  }

  return (
    <DetailPageSection
      title="Your customizations"
      description="Reset anything you've rearranged around the dashboard."
    >
      <div className="divide-y overflow-hidden rounded-md border">
        <Row
          title="Home layout"
          detail={homeKeys.length ? 'Widgets reordered or hidden' : 'Untouched'}
          disabled={homeKeys.length === 0}
          onReset={() => remove(homeKeys)}
        />
        <Row
          title="Pinned pages"
          detail={pins.length ? `${pins.length} pinned` : 'None pinned'}
          label="Clear all"
          disabled={pins.length === 0}
          onReset={() => remove(['nav:pins'])}
        />
      </div>

      <div className="grid gap-1.5">
        <p className="text-sm font-medium">Saved views</p>
        {viewKeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">No saved views yet.</p>
        ) : (
          <div className="divide-y overflow-hidden rounded-md border">
            {viewKeys.map((key) => (
              <ViewsRow key={key} prefKey={key} onRemoveKey={() => remove([key])} />
            ))}
          </div>
        )}
      </div>

      <div className="grid gap-1.5">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm font-medium">Table layouts</p>
          {tableKeys.length > 1 && (
            <ConfirmButton
              label="Reset all"
              title="Reset every table layout?"
              body="All lists go back to their default columns, widths and density."
              onConfirm={() => remove(tableKeys)}
            />
          )}
        </div>
        {tableKeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No table layouts saved - lists use their defaults.
          </p>
        ) : (
          <div className="divide-y overflow-hidden rounded-md border">
            {tableKeys.map((key) => (
              <Row
                key={key}
                title={key.slice('table:'.length)}
                detail="Columns, widths and density"
                onReset={() => remove([key])}
              />
            ))}
          </div>
        )}
      </div>
    </DetailPageSection>
  )
}

// Each key needs its own usePreference, and hooks can't run in a loop over a changing list
function ViewsRow({ prefKey, onRemoveKey }: { prefKey: string; onRemoveKey: () => void }) {
  const [store, setStore] = usePreference<ViewsPref>(prefKey, EMPTY_VIEWS)
  const names = store.views.map((v) => v.name).join(', ')

  return (
    <div className="flex items-center gap-3 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{prefKey.slice('views:'.length)}</p>
        <p className="truncate text-xs text-muted-foreground">{names || 'No views'}</p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {store.views.map((view) => (
          <Button
            key={view.id}
            variant="ghost"
            size="sm"
            title={`Delete "${view.name}"`}
            onClick={() =>
              setStore((prev) => ({
                views: prev.views.filter((v) => v.id !== view.id),
                defaultId: prev.defaultId === view.id ? undefined : prev.defaultId,
              }))
            }
          >
            <Trash2Icon className="size-3.5" />
            {view.name}
          </Button>
        ))}
        <ConfirmButton
          label="Delete all"
          title="Delete every saved view for this list?"
          body="The list falls back to no default view."
          onConfirm={onRemoveKey}
        />
      </div>
    </div>
  )
}

function Row({
  title,
  detail,
  label = 'Reset',
  disabled,
  onReset,
}: {
  title: string
  detail: string
  label?: string
  disabled?: boolean
  onReset: () => void
}) {
  return (
    <div className="flex items-center gap-3 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{title}</p>
        <p className="truncate text-xs text-muted-foreground">{detail}</p>
      </div>
      <ConfirmButton
        label={label}
        title={`${label} ${title.toLowerCase()}?`}
        body="This only affects your own account."
        disabled={disabled}
        onConfirm={onReset}
      />
    </div>
  )
}

function ConfirmButton({
  label,
  title,
  body,
  disabled,
  onConfirm,
}: {
  label: string
  title: string
  body: string
  disabled?: boolean
  onConfirm: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="ghost" size="sm" disabled={disabled}>
          {label}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm}>{label}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
