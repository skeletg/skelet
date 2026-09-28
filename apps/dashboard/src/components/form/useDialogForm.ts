import { useState, type FormEvent } from 'react'
import type { StandardSchemaV1 } from '@tanstack/react-form'
import { submitForm } from '@/shared/saving/useFormSection'
import { useAppForm } from './form'

// A create/edit dialog's form - the CRUD-dialog exception to the single header Save, so it writes
// on its own submit. Call it from a component rendered INSIDE DialogContent: Radix unmounts the
// content on close, so every open starts from `baseline` with no re-hydrate dance
//
// Render the result with FormDialogBody (./FormDialogBody)

export function useDialogForm<T extends object>(opts: {
  baseline: T
  schema: StandardSchemaV1<T, unknown>
  /** The mutation - throw on failure, the message shows in the dialog */
  submit: (values: T) => Promise<void>
  /** After a successful submit - close the dialog, refetch, navigate */
  onSaved?: () => void
}) {
  const [error, setError] = useState<string | null>(null)
  const form = useAppForm({
    defaultValues: opts.baseline,
    validators: { onChange: opts.schema },
    onSubmit: async ({ value }) => {
      await opts.submit(value)
      opts.onSaved?.()
    },
  })

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    // The dialog portals out of the DOM but not out of the React tree - without this, a dialog
    // opened from inside another form also submits that one
    e.stopPropagation()
    setError(null)
    try {
      await submitForm(form)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  return { form, error, onSubmit }
}
