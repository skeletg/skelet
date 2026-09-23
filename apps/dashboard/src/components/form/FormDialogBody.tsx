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
}: {
  dialog: { form: AnyFormApi; error: string | null; onSubmit: (e: FormEvent<HTMLFormElement>) => void }
  title: ReactNode
  description?: ReactNode
  submitLabel: string
  onCancel: () => void
  children: ReactNode
}) {
  const submitting = useStore(dialog.form.store, (s) => s.isSubmitting)
  return (
    <form onSubmit={dialog.onSubmit} className="grid gap-4" noValidate>
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        {description && <DialogDescription>{description}</DialogDescription>}
      </DialogHeader>
      {children}
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
        <Button type="submit" disabled={submitting}>
          {submitting ? 'Saving…' : submitLabel}
        </Button>
      </DialogFooter>
    </form>
  )
}
