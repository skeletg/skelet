import type { StandardSchemaV1 } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import type { SaveRegistryValue, SectionDiffEntry } from './saveRegistry'
import { useFormSection } from './useFormSection'
import { usePersistedForm } from './usePersistedForm'

// One section of a detail page that stages several - a TanStack form with its own stored draft,
// registered into the page's save registry through context, so the page's one header Save reviews
// and writes it alongside the others. For a page that IS one form, use useRecordForm instead
//
// Render the draft banner inside the section: `<RestoredDraftBanner draft={draft} labels={…} />` - it
// offers a stored draft for review and renders nothing when there is none

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
  /** The page's own registry, when this runs in the component that renders its Provider */
  registry?: SaveRegistryValue
  /** False lists nothing and keeps no draft - a read-only view of the section */
  enabled?: boolean
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
  const enabled = opts.enabled ?? true
  const draft = usePersistedForm(form, { routeKey: opts.routeKey, baseline: opts.baseline, label: opts.label, enabled, diff: opts.diff })
  useFormSection({
    id: opts.id,
    title: opts.title,
    form,
    baseline: opts.baseline,
    diff: opts.diff,
    enabled,
    registry: opts.registry,
    persisted: true,
  })
  return { form, draft }
}
