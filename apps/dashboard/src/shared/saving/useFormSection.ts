import { useStore, type AnyFormApi } from '@tanstack/react-form'
import type { SaveRegistryValue, SectionDiffEntry } from './saveRegistry'
import { useSaveSection } from './saveRegistry'

// A TanStack form as one staged-save section - the form owns values and validation, this turns it
// into the registry's {diff, save} pair so the page-header Save and SaveConfirmDialog work as on
// every other detail page (docs/conventions.md)
//
// `baseline` is passed in rather than read off form.options: useForm re-applies its options in a
// layout effect AFTER render, so a refetched record would show up in the diff a render late

export type FormSectionOptions<T> = {
  id: string
  title: string
  form: AnyFormApi
  baseline: T
  /** Build with fieldDiff / changed / added / removed from ./diff - one entry per real change */
  diff: (baseline: T, values: T) => SectionDiffEntry[]
  /** False lists nothing - a create form has no record to compare against */
  enabled?: boolean
  /** The page's own registry, when this runs in the component that calls useSaveRegistry */
  registry?: SaveRegistryValue
}

export function useFormSection<T>({ id, title, form, baseline, diff, enabled = true, registry }: FormSectionOptions<T>) {
  const values = useStore(form.store, (s) => s.values as T)
  useSaveSection(
    {
      id,
      title,
      diff: enabled ? diff(baseline, values) : [],
      save: () => submitForm(form),
    },
    registry,
  )
}

// handleSubmit resolves quietly when validation fails - saveAll needs a rejection so the confirm
// dialog stays open with the reason. Errors thrown by onSubmit (the mutation) already propagate
export async function submitForm(form: AnyFormApi): Promise<void> {
  await form.handleSubmit()
  if (!form.state.isSubmitSuccessful) {
    throw new Error(firstFieldError(form) ?? 'Fix the highlighted fields first.')
  }
}

function firstFieldError(form: AnyFormApi): string | null {
  for (const [name, meta] of Object.entries(form.state.fieldMeta)) {
    const first: unknown = (meta as { errors?: unknown[] } | undefined)?.errors?.[0]
    if (!first) continue
    const message =
      typeof first === 'string'
        ? first
        : typeof first === 'object' && 'message' in first
          ? String(first.message)
          : null
    if (message) return `${name}: ${message}`
  }
  return null
}
