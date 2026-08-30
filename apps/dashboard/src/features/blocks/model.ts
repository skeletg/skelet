import type { BlockComponent, CustomFieldDef, EmailBlock } from '@/features/email/blocks'
import type { EmailBlockFieldsFragment } from '@/features/email/Email.gql'

// Maps the GraphQL EmailBlockSnippet, whose `fields`/`blocks` ride the JSON scalar as `any`, into the typed client model
export function toComponent(b: EmailBlockFieldsFragment): BlockComponent {
  return {
    slug: b.slug,
    name: b.name,
    icon: b.icon || undefined,
    fields: (b.fields as CustomFieldDef[] | null) ?? [],
    blocks: (b.blocks as EmailBlock[] | null) ?? [],
    htmlTemplate: b.htmlTemplate || undefined,
    css: b.css || undefined,
    scripted: b.scripted || undefined,
  }
}
