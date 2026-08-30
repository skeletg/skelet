import { ArrowDownIcon, ArrowUpIcon, EyeIcon, EyeOffIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  EMPTY_LAYOUT,
  applyLayout,
  isHiddenInLayout,
  moveInLayout,
  toggleInLayout,
  type OrderedLayout,
} from '@/lib/layout-pref'
import { usePreference } from '@/lib/admin-prefs'

// defaultHidden flips the item's resting state: it stays off until switched on, which is how the
// Home section library and the reporting-grade tiles work
export type ArrangeItem = { id: string; label: string; hint?: string; defaultHidden?: boolean }

// The reorder-and-hide list Preferences uses for anything arrangeable - same storage shape and the
// same up/down/eye controls as Home's own customize mode, so an admin only learns it once
export function ArrangeList({
  prefKey,
  items,
  emptyLabel = 'Nothing to arrange for this account.',
}: {
  prefKey: string
  items: ArrangeItem[]
  emptyLabel?: string
}) {
  const [layout, setLayout] = usePreference<OrderedLayout>(prefKey, EMPTY_LAYOUT)
  const ordered = applyLayout(items, layout)
  const ids = ordered.map((i) => i.id)
  const touched =
    layout.order.length > 0 || layout.hidden.length > 0 || (layout.shown ?? []).length > 0

  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">{emptyLabel}</p>
  }

  return (
    // capped width keeps the row controls clear of the floating section index on the right edge
    <div className="max-w-3xl space-y-2">
      <div className="divide-y rounded-md border">
        {ordered.map((item, i) => {
          const hidden = isHiddenInLayout(layout, item.id, item.defaultHidden)
          return (
            <div key={item.id} className="flex items-center gap-3 px-3 py-2">
              {/* the accent bar is the at-a-glance answer to "what did I turn off" */}
              <span
                className={cn(
                  'h-8 w-0.5 shrink-0 rounded-full',
                  hidden ? 'bg-transparent' : 'bg-primary/40',
                )}
              />
              <span className="min-w-0 flex-1">
                <span className={cn('block text-sm', hidden && 'text-muted-foreground line-through')}>
                  {item.label}
                </span>
                {item.hint && (
                  <span className="block text-xs text-muted-foreground">{item.hint}</span>
                )}
              </span>
              <span className="flex items-center gap-0.5">
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  disabled={i === 0}
                  onClick={() => setLayout((prev) => moveInLayout(prev, ids, item.id, -1))}
                  aria-label={`Move ${item.label} up`}
                >
                  <ArrowUpIcon className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  disabled={i === ordered.length - 1}
                  onClick={() => setLayout((prev) => moveInLayout(prev, ids, item.id, 1))}
                  aria-label={`Move ${item.label} down`}
                >
                  <ArrowDownIcon className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  onClick={() => setLayout((prev) => toggleInLayout(prev, item.id, item.defaultHidden))}
                  aria-label={hidden ? `Show ${item.label}` : `Hide ${item.label}`}
                >
                  {hidden ? <EyeOffIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
                </Button>
              </span>
            </div>
          )
        })}
      </div>
      {touched && (
        <Button variant="ghost" size="sm" onClick={() => setLayout(undefined)}>
          Reset to default
        </Button>
      )}
    </div>
  )
}
