import type { FormEvent, ReactNode } from 'react'
import { useStore, type AnyFormApi } from '@tanstack/react-form'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

// The inside of a useDialogForm dialog - header, fields, the failure alert and the Cancel/submit
// footer - so each dialog is only its fields. Put it inside DialogContent
export function FormDialogBody({
  dialog,
  title,
  description,
  submitLabel,
  onCancel,
  children,
  scrollable = false,
  canSubmit = true,
}: {
  dialog: { form: AnyFormApi; error: string | null; onSubmit: (e: FormEvent<HTMLFormElement>) => void }
  title: ReactNode
  description?: ReactNode
  submitLabel: string
  onCancel: () => void
  children: ReactNode
  /** Long forms: the fields scroll between a fixed header and footer - pair with a max-h on the DialogContent */
  scrollable?: boolean
  /** False disables the submit button - nothing to send yet */
  canSubmit?: boolean
}) {
  const submitting = useStore(dialog.form.store, (s) => s.isSubmitting)
  return (
    <form
      onSubmit={dialog.onSubmit}
      className={scrollable ? 'flex min-h-0 flex-1 flex-col gap-4' : 'grid gap-4'}
      noValidate
    >
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        {description && <DialogDescription>{description}</DialogDescription>}
      </DialogHeader>
      {scrollable ? <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto py-1 pr-1">{children}</div> : children}
      {dialog.error && (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{dialog.error}</AlertDescription>
        </Alert>
      )}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={submitting || !canSubmit}>
          {submitting ? 'Saving…' : submitLabel}
        </Button>
      </DialogFooter>
    </form>
  )
}
