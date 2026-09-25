import type { StandardSchemaV1 } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import type { SectionDiffEntry } from './saveRegistry'
import { useFormSection } from './useFormSection'
import { usePersistedForm } from './usePersistedForm'

// One section of a detail page that stages several - a TanStack form with its own stored draft,
// registered into the page's save registry through context, so the page's one header Save reviews
// and writes it alongside the others. For a page that IS one form, use useRecordForm instead
//
// Render the draft banner inside the section: `{draft.restoredAt !== null && <RestoredDraftBanner …/>}`

export function useSectionForm<T extends object>(opts: {
  /** Section id and title in the confirm dialog */
  id: string
  title: string
  /** The stored record as form values - memoise it on the record */
  baseline: T
  schema: StandardSchemaV1<T, unknown>
  diff: (baseline: T, values: T) => SectionDiffEntry[]
  /** Stored-draft key, e.g. `cms-page-basics:${id}` */
  routeKey: string
  /** Names the page in the close-tab prompt */
  label?: string
  /** The mutations - throw on failure, the confirm dialog shows the message */
  submit: (values: T) => Promise<void>
}) {
  const form = useAppForm({
    defaultValues: opts.baseline,
    validators: { onChange: opts.schema },
    onSubmit: async ({ value }) => {
      await opts.submit(value)
      draft.saved()
      form.reset(value)
    },
  })
  const draft = usePersistedForm(form, { routeKey: opts.routeKey, baseline: opts.baseline, label: opts.label })
  useFormSection({ id: opts.id, title: opts.title, form, baseline: opts.baseline, diff: opts.diff, persisted: true })
  return { form, draft }
}
