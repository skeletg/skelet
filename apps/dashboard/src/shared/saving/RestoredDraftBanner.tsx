import { HistoryIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { display } from './diff'
import type { PersistedForm } from './usePersistedForm'

// Says out loud that the form came back from a stored draft - silently refilled fields read as
// "the save worked" when nothing has been saved yet. Neutral/amber tokens on purpose: primary and
// accent are admin-recolourable and never carry state (docs/conventions.md)
export function RestoredDraftBanner({
  draft,
  labels = {},
}: {
  draft: PersistedForm
  /** Field name -> what the page calls it, for the clash list */
  labels?: Record<string, string>
}) {
  if (draft.restoredAt === null) return null
  const at = new Date(draft.restoredAt).toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  return (
    <div className="flex items-start gap-3 rounded-md border border-warning/40 bg-warning-soft px-4 py-3 text-sm">
      <HistoryIcon className="mt-0.5 size-4 shrink-0 text-warning" />
      <div className="grid flex-1 gap-1">
        <p>
          Restored unsaved changes from <span className="font-medium">{at}</span>. They aren&apos;t saved
          until you press Save.
        </p>
        {draft.clashes.length > 0 && (
          <div className="grid gap-0.5 text-xs text-muted-foreground">
            <p>
              {draft.clashes.length === 1
                ? '1 field was also changed by someone else since then - your value is kept:'
                : `${draft.clashes.length} fields were also changed by someone else since then - your values are kept:`}
            </p>
            <ul className="list-disc pl-4">
              {draft.clashes.map((c) => (
                <li key={c.field}>
                  {labels[c.field] ?? c.field} - server now: {display(c.server) || '—'}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
      <Button type="button" variant="outline" size="sm" onClick={draft.discard}>
        Discard
      </Button>
    </div>
  )
}

/** The per-field note under a clashing input */
export function ClashNote({ draft, field }: { draft: PersistedForm; field: string }) {
  const clash = draft.clashes.find((c) => c.field === field)
  if (!clash) return null
  return (
    <p className="text-xs text-warning">
      Server now: {display(clash.server) || '—'}
    </p>
  )
}
