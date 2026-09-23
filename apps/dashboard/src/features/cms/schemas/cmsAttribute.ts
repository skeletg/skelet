import { z } from 'zod'
import type { CmsInputType, CmsRelationshipTarget } from '@/graphql/schema.types'

// New CMS attribute - a custom field definition page types reuse. Slug and input type are fixed
// after creation; the display name is seeded for the locale being edited

export const CMS_INPUT_TYPES: { value: CmsInputType; label: string }[] = [
  { value: 'TEXT', label: 'Text (single line)' },
  { value: 'TEXTAREA', label: 'Text (multi-line)' },
  { value: 'RICHTEXT', label: 'Rich text (Lexical)' },
  { value: 'NUMBER', label: 'Number' },
  { value: 'BOOLEAN', label: 'Boolean' },
  { value: 'DATE', label: 'Date' },
  { value: 'DATETIME', label: 'Date + time' },
  { value: 'URL', label: 'URL' },
  { value: 'SELECT', label: 'Select (one option)' },
  { value: 'MULTISELECT', label: 'Multi-select' },
  { value: 'ASSET', label: 'Media asset (one)' },
  { value: 'ASSETS', label: 'Media assets (many)' },
  { value: 'RELATIONSHIP', label: 'Relationship' },
  { value: 'JSON', label: 'JSON (free-form)' },
]

export const CMS_RELATIONSHIP_TARGETS: CmsRelationshipTarget[] = ['CMS_PAGE', 'BLOG_POST', 'PRODUCT', 'COLLECTION', 'CATEGORY']

export const newCmsAttributeSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required'),
    name: z.string().trim().min(1, 'Display name is required'),
    inputType: z.string(),
    translatable: z.boolean(),
    requiredDefault: z.boolean(),
    unit: z.string(),
    // '' until picked - only a relationship needs one
    target: z.string(),
  })
  .refine((v) => v.inputType !== 'RELATIONSHIP' || v.target !== '', {
    message: 'A relationship needs something to point at',
    path: ['target'],
  })

export type NewCmsAttributeValues = z.infer<typeof newCmsAttributeSchema>

export const emptyNewCmsAttribute: NewCmsAttributeValues = {
  slug: '',
  name: '',
  inputType: 'TEXT',
  translatable: false,
  requiredDefault: false,
  unit: '',
  target: '',
}

export function toNewCmsAttributeInput(v: NewCmsAttributeValues) {
  return {
    slug: v.slug.trim(),
    inputType: v.inputType as CmsInputType,
    isTranslatable: v.translatable,
    isRequiredDefault: v.requiredDefault,
    // A unit only means something on a number
    unit: v.inputType === 'NUMBER' ? v.unit.trim() || null : null,
    relationshipTarget: v.inputType === 'RELATIONSHIP' && v.target ? (v.target as CmsRelationshipTarget) : null,
  }
}
