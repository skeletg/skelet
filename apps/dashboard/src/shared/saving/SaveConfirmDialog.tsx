import { Trans, useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import type { SectionDiffEntry } from './saveRegistry'

export type { SectionDiffEntry }

export type ChangeSection = {
  id: string
  title: string
  diff: SectionDiffEntry[]
}

/** Generic "review pending changes before saving" modal - The caller computes
 *  one or more `ChangeSection`s, each grouping a few diff rows (label / old /
 *  new), and the dialog renders them in a divided list - The active scope is
 *  shown at the top so admins know what context the writes will land in */
export function SaveConfirmDialog({
  open,
  scopeLabel,
  sections,
  saving,
  error,
  onCancel,
  onConfirm,
}: {
  open: boolean
  scopeLabel: string
  sections: ChangeSection[]
  saving: boolean
  error: string | null
  onCancel: () => void
  onConfirm: () => void
}) {
  const { t } = useLingui()
  const totalChanges = sections.reduce((n, s) => n + s.diff.length, 0)
  return (
    <AlertDialog open={open} onOpenChange={(v) => { if (!v) onCancel() }}>
      <AlertDialogContent className="sm:max-w-xl">
        <AlertDialogHeader>
          <AlertDialogTitle>{plural(totalChanges, { one: 'Save # change?', other: 'Save # changes?' })}</AlertDialogTitle>
          <AlertDialogDescription>
            <Trans>Review the pending updates below before applying them.</Trans>
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className="grid gap-3">
          <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-xs">
            <span className="font-medium uppercase tracking-wide text-muted-foreground"><Trans>Scope</Trans></span>
            <span className="font-semibold">{scopeLabel}</span>
          </div>

          <div className="scrollbar-accent max-h-[18rem] overflow-y-auto rounded-md border">
            {sections.length === 0 ? (
              <p className="p-3 text-xs text-muted-foreground"><Trans>No pending changes.</Trans></p>
            ) : (
              <ul className="divide-y">
                {sections.map((section) => (
                  <li key={section.id} className="px-3 py-2.5">
                    <p className="mb-1.5 text-xs font-semibold tracking-tight">{section.title}</p>
                    <ul className="grid gap-1">
                      {section.diff.map((d, i) => (
                        <li key={i} className="grid grid-cols-[7rem_1fr] gap-2 text-xs">
                          <span className="text-muted-foreground">{d.label}</span>
                          {d.kind === 'add' ? (
                            <span className="min-w-0 break-all">
                              <span className="mr-1.5 font-semibold text-success">+</span>
                              <span className="font-medium">{d.after || '(new)'}</span>
                            </span>
                          ) : d.kind === 'remove' ? (
                            <span className="min-w-0 break-all">
                              <span className="mr-1.5 font-semibold text-destructive">−</span>
                              <span className="text-muted-foreground line-through">{d.before || d.label}</span>
                            </span>
                          ) : (
                            <span className="min-w-0">
                              <span className="break-all text-muted-foreground line-through">{d.before || '-'}</span>
                              <span className="mx-1.5 text-muted-foreground">→</span>
                              <span className="break-all font-medium">{d.after || '-'}</span>
                            </span>
                          )}
                        </li>
                      ))}
                    </ul>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertTitle><Trans>Save failed</Trans></AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel disabled={saving}><Trans>Cancel</Trans></AlertDialogCancel>
          <AlertDialogAction
            disabled={saving || sections.length === 0}
            onClick={(e) => { e.preventDefault(); onConfirm() }}
          >
            {saving ? t`Saving…` : plural(totalChanges, { one: 'Apply # change', other: 'Apply # changes' })}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
