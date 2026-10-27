import { z } from 'zod'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { currencyCode } from '@/shared/schemas/fields'

// Configurator template - a named set of options and rules many products share

export type ExistingOptionTemplate = { id: string; name: string; description: string; currencyCode: string }

export const optionTemplateSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required').max(120, 'At most 120 characters').describe('Name admins pick the template by, unique across templates'),
    description: z.string().max(2000, 'At most 2000 characters').describe('What the template is for - shown only in the admin'),
    currencyCode: currencyCode().describe('Currency a flat charge on its options and rules is in - per-store amounts cover the other currencies'),
  })
  .describe('A configurator template')

export type OptionTemplateValues = z.infer<typeof optionTemplateSchema>

export function toOptionTemplateValues(t?: ExistingOptionTemplate): OptionTemplateValues {
  return { name: t?.name ?? '', description: t?.description ?? '', currencyCode: t?.currencyCode ?? 'USD' }
}

export function toOptionTemplateInput(v: OptionTemplateValues) {
  return { name: v.name.trim(), description: v.description.trim(), currencyCode: v.currencyCode }
}

export const OPTION_TEMPLATE_LABELS: Record<keyof OptionTemplateValues, string> = {
  name: 'Name',
  description: 'Description',
  currencyCode: 'Currency',
}

export function optionTemplateDiff(original: OptionTemplateValues, draft: OptionTemplateValues): SectionDiffEntry[] {
  return fieldDiff(
    original,
    draft,
    (['name', 'description', 'currencyCode'] as const).map((k) => ({ label: OPTION_TEMPLATE_LABELS[k], value: (v: OptionTemplateValues) => v[k] })),
  )
}
