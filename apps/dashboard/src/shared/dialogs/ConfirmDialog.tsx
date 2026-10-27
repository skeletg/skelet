import { useState, type ReactNode } from 'react'
import { duringCommit } from '@/lib/nav-guard'
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

// The same 38-line confirm block was hand-written in ~63 files, each repeating the destructive classes, a busy label and its own error state
// onConfirm is awaited - throw to keep the dialog open and show the message, return to close it, which is what lets callers drop their own busy and error state
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  busyLabel,
  destructive = false,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  confirmLabel?: ReactNode
  cancelLabel?: ReactNode // 'Keep' where the action itself is a cancel, so the two buttons don't both say it
  busyLabel?: ReactNode // defaults to the confirm label plus an ellipsis, what nearly every caller wrote by hand
  destructive?: boolean
  onConfirm: () => void | Promise<void>
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // A reopened dialog must not still show the last attempt's failure - adjusted during render rather than in an effect, which would commit the stale error for a frame
  const [lastOpen, setLastOpen] = useState(open)
  if (open !== lastOpen) {
    setLastOpen(open)
    if (open) setError(null)
  }

  async function run() {
    setBusy(true)
    setError(null)
    try {
      // The admin already confirmed THIS action - handlers that navigate on success (every
      // delete-then-back-to-the-list) must not then be asked to confirm leaving as well
      await duringCommit(onConfirm)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Action failed.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(next) => !busy && onOpenChange(next)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn&apos;t complete</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction
            disabled={busy}
            // preventDefault keeps Radix from closing before the mutation resolves
            onClick={(e) => {
              e.preventDefault()
              void run()
            }}
            className={
              destructive
                ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90'
                : undefined
            }
          >
            {busy ? (busyLabel ?? <>{confirmLabel}…</>) : confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
