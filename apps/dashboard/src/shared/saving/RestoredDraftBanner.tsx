import { useState } from 'react'
import { HistoryIcon } from 'lucide-react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { display } from './diff'
import { DiffRows } from './SaveConfirmDialog'
import type { PersistedForm } from './usePersistedForm'

// Offers a stored draft instead of refilling the form behind the admin's back: says one was found,
// and Review lists exactly what restoring would change - the rows the Save dialog would show -
// before anything moves. Warning tokens on purpose: primary and accent are admin-recolourable and
// never carry state (docs/conventions.md). Renders nothing when there is no draft on offer
export function RestoredDraftBanner({
  draft,
  labels = {},
}: {
  draft: PersistedForm
  /** Field name -> what the page calls it, for the clash list and field-keyed rows */
  labels?: Record<string, string>
}) {
  const [reviewing, setReviewing] = useState(false)
  const offer = draft.offer
  if (!offer) return null
  const at = new Date(offer.at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  const count = offer.rows.length
  const changes = count === 1 ? '1 change' : `${count} changes`

  return (
    <div className="flex items-start gap-3 rounded-md border border-warning/40 bg-warning-soft px-4 py-3 text-sm">
      <HistoryIcon className="mt-1.5 size-4 shrink-0 text-warning" />
      <p className="flex-1 py-1">
        Unsaved changes from <span className="font-medium">{at}</span> found - {changes}. Review them before they go back into
        the form.
      </p>
      <div className="flex shrink-0 gap-2">
        <Button type="button" variant="outline" size="sm" onClick={() => setReviewing(true)}>
          Review
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={draft.discard}>
          Discard
        </Button>
      </div>

      <AlertDialog open={reviewing} onOpenChange={setReviewing}>
        <AlertDialogContent className="sm:max-w-xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{count === 1 ? 'Restore 1 unsaved change?' : `Restore ${count} unsaved changes?`}</AlertDialogTitle>
            <AlertDialogDescription>
              From {at}. Restoring puts them back into the form - they still need Save to reach the store.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid gap-3">
            <div className="scrollbar-accent max-h-72 overflow-y-auto rounded-md border px-3 py-2.5">
              <DiffRows entries={offer.rows.map((r) => ({ ...r, label: labels[r.label] ?? r.label }))} />
            </div>
            {offer.clashes.length > 0 && (
              <div className="grid gap-0.5 rounded-md border border-warning/40 bg-warning-soft px-3 py-2 text-xs">
                <p>
                  {offer.clashes.length === 1
                    ? '1 field was also changed by someone else since then - restoring keeps your value:'
                    : `${offer.clashes.length} fields were also changed by someone else since then - restoring keeps your values:`}
                </p>
                <ul className="list-disc pl-4 text-muted-foreground">
                  {offer.clashes.map((c) => (
                    <li key={c.field}>
                      {labels[c.field] ?? c.field} - now {display(c.server) || '—'}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel
              onClick={() => {
                setReviewing(false)
                draft.discard()
              }}
            >
              Discard
            </AlertDialogCancel>
            <Button
              type="button"
              onClick={() => {
                setReviewing(false)
                draft.restore()
              }}
            >
              Restore
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

/** The per-field note under a restored input the server moved meanwhile */
export function ClashNote({ draft, field }: { draft: PersistedForm; field: string }) {
  const clash = draft.clashes.find((c) => c.field === field)
  if (!clash) return null
  return <p className="text-xs text-warning">Server now: {display(clash.server) || '—'}</p>
}
