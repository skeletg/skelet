import { useEffect, useState } from 'react'
import type { StandardSchemaV1 } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import { duringCommit } from '@/lib/nav-guard'
import type { SectionDiffEntry } from './saveRegistry'
import { useSaveRegistry } from './saveRegistry'
import { useSaveConfirm } from './useSaveConfirm'
import { submitForm, useFormSection } from './useFormSection'
import { usePersistedForm } from './usePersistedForm'

// One record, one form, one page - the wiring every create/edit page repeats, in one place:
// the page's save registry, the TanStack form with its zod schema, the stored draft, the section the
// header Save reviews, the confirm dialog, and the create path that writes straight through
//
// The page supplies what's specific to it - schema, baseline, diff and the mutations - and renders
// with RecordFormPage (./RecordFormPage), which lays out the header, banners and dialog around it

export type RecordFormOptions<T extends object> = {
  /** Section id and title in the confirm dialog - 'country', 'Country' */
  id: string
  title: string
  /** Names the page in discard prompts and the dialog - `Country · Germany` */
  label: string
  isEdit: boolean
  /** The stored record as form values - memoise it on the record, it's the diff and draft baseline */
  baseline: T
  schema: StandardSchemaV1<T, unknown>
  diff: (baseline: T, values: T) => SectionDiffEntry[]
  /** Stored-draft key, e.g. `country:${code ?? 'new'}` - false keeps no draft (dialogs) */
  persist: { routeKey: string; exclude?: readonly string[] } | false
  /** The mutations - throw on failure, the dialog or the create alert shows the message */
  submit: (values: T) => Promise<void>
  /** After a successful save - usually navigating back to the list; runs inside duringCommit */
  onSaved?: () => void | Promise<void>
  /** Routes a server error message to the field it's about - unmatched messages stay general */
  fieldErrorsFrom?: (message: string) => Partial<Record<keyof T & string, string>>
}

export function useRecordForm<T extends object>(opts: RecordFormOptions<T>) {
  const { isEdit, baseline, persist } = opts
  const registry = useSaveRegistry(opts.label)
  const confirm = useSaveConfirm(registry.saveAll)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const form = useAppForm({
    defaultValues: baseline,
    validators: { onChange: opts.schema },
    onSubmit: async ({ value }) => {
      try {
        await opts.submit(value)
      } catch (err) {
        routeServerError(err)
        throw err
      }
      draft.saved()
      form.reset(value)
      if (opts.onSaved) await duringCommit(opts.onSaved)
    },
  })

  const draft = usePersistedForm(form, {
    routeKey: persist ? persist.routeKey : '',
    exclude: persist ? persist.exclude : [],
    baseline,
    label: opts.label,
    enabled: persist !== false,
    diff: opts.diff,
  })

  // A schema built from loaded data (a default locale, say) changes after the admin may already have typed.
  // TanStack keeps the error the old schema raised until the next edit, so a create blocked on "Pick a locale"
  // while the field showed one - run the new schema over what's there
  useEffect(() => {
    if (form.state.isTouched) void form.validate('change')
  }, [form, opts.schema])

  // Registered straight into the page's own registry - this hook runs in the component that renders
  // the Provider, so context would come back empty. A create has nothing to compare against
  useFormSection({
    id: opts.id,
    title: opts.title,
    form,
    baseline,
    diff: opts.diff,
    enabled: isEdit,
    registry: registry.contextValue,
    persisted: persist !== false,
  })

  // Server-side rejections that name a field show under that field, like a schema error would
  function routeServerError(err: unknown) {
    if (!opts.fieldErrorsFrom) return
    const message = err instanceof Error ? err.message : String(err)
    for (const [field, text] of Object.entries(opts.fieldErrorsFrom(message))) {
      if (!text) continue
      form.setFieldMeta(field as never, (prev) => ({
        ...prev,
        isTouched: true,
        errorMap: { ...prev.errorMap, onSubmit: text },
      }))
    }
  }

  // Create writes straight through - a brand new record has no before to review
  async function create() {
    setSubmitError(null)
    try {
      await submitForm(form)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return {
    form,
    draft,
    registry,
    confirm,
    submitError,
    /** The header Save: review the diff when editing, write straight through when creating */
    save: () => (isEdit ? confirm.open() : void create()),
    isEdit,
    label: opts.label,
  }
}

export type RecordForm = ReturnType<typeof useRecordForm<object>>
