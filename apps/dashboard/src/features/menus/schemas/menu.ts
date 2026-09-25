import { z } from 'zod'
import { handleSlug } from '@/shared/schemas/fields'
import type { MenuItemTargetType } from '@/graphql/schema.types'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, fieldDiff, removed } from '@/shared/saving/diff'

// Menu page - THE field list (docs/conventions.md, "Record forms"): the menu's slug, name and the
// channels serving it. Items are rows with their own dialog (below), not fields of the page

export const menuSchema = z.object({
  slug: handleSlug(),
  name: z.string().trim().min(1, 'Name is required'),
  channelIds: z.array(z.string()),
})

export type MenuValues = z.infer<typeof menuSchema>

export const MENU_LABELS: Record<keyof MenuValues, string> = { slug: 'Slug', name: 'Name', channelIds: 'Channels' }

export function toMenuValues(menu: { slug: string; name: string; channelIds?: string[] | null }): MenuValues {
  return { slug: menu.slug, name: menu.name, channelIds: menu.channelIds ?? [] }
}

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x))

// The menu's writes, each only when its part changed
export function menuChanges(before: MenuValues, after: MenuValues) {
  return {
    update: before.slug !== after.slug || before.name !== after.name ? { slug: after.slug.trim(), name: after.name.trim() } : null,
    channelIds: sameSet(before.channelIds, after.channelIds) ? null : after.channelIds,
  }
}

export function menuDiff(channelName: (id: string) => string) {
  return (original: MenuValues, draft: MenuValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      { label: MENU_LABELS.slug, value: (v) => v.slug },
      { label: MENU_LABELS.name, value: (v) => v.name },
    ]),
    ...draft.channelIds.filter((id) => !original.channelIds.includes(id)).map((id) => added(`Channel · ${channelName(id)}`)),
    ...original.channelIds.filter((id) => !draft.channelIds.includes(id)).map((id) => removed(`Channel · ${channelName(id)}`)),
  ]
}

// Menu item dialog - one select picks "a URL" or an entity kind; the backend takes either a url or a
// typed target, never both, so the other side is cleared on the way out

export const TARGET_KINDS: { value: 'URL' | MenuItemTargetType; label: string }[] = [
  { value: 'URL', label: 'External / custom URL' },
  { value: 'PRODUCT', label: 'Product' },
  { value: 'CATEGORY', label: 'Category' },
  { value: 'COLLECTION', label: 'Collection' },
  { value: 'BLOG_POST', label: 'Blog post' },
  { value: 'BLOG_CATEGORY', label: 'Blog category' },
  { value: 'BLOG_TAG', label: 'Blog tag' },
  { value: 'BLOG_AUTHOR', label: 'Blog author' },
  { value: 'CMS_PAGE', label: 'CMS page' },
]

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export const menuItemSchema = z
  .object({
    label: z.string().trim().min(1, 'Label is required'),
    kind: z.string(),
    url: z.string(),
    targetId: z.string(),
    // '' = top level
    parentId: z.string(),
    // Locale -> translated label; an emptied row is removed on save
    translations: z.record(z.string(), z.string()),
  })
  .superRefine((v, ctx) => {
    if (v.kind === 'URL') {
      if (!v.url.trim()) ctx.addIssue({ code: 'custom', path: ['url'], message: 'URL is required' })
    } else if (!UUID.test(v.targetId.trim())) {
      ctx.addIssue({ code: 'custom', path: ['targetId'], message: "Paste the target's ID (a UUID)" })
    }
  })

export type MenuItemValues = z.infer<typeof menuItemSchema>

type ExistingItem = {
  label: string
  targetType: MenuItemTargetType | null
  targetId: string | null
  url: string | null
  parentId: string | null
  translations: { locale: string; label: string }[]
}

export function toMenuItemValues(item?: ExistingItem, parentId?: string | null): MenuItemValues {
  return {
    label: item?.label ?? '',
    kind: item?.targetType ?? 'URL',
    url: item?.url ?? '',
    targetId: item?.targetId ?? '',
    parentId: item?.parentId ?? parentId ?? '',
    translations: Object.fromEntries((item?.translations ?? []).map((t) => [t.locale, t.label])),
  }
}

export function toMenuItemInput(v: MenuItemValues) {
  const isUrl = v.kind === 'URL'
  return {
    label: v.label.trim(),
    targetType: isUrl ? null : (v.kind as MenuItemTargetType),
    targetId: isUrl ? null : v.targetId.trim(),
    url: isUrl ? v.url.trim() : null,
    parentId: v.parentId || null,
  }
}

// Translation writes: labels set or changed, and locales removed or emptied
export function translationChanges(before: MenuItemValues, after: MenuItemValues) {
  const upserts = Object.entries(after.translations)
    .filter(([locale, label]) => label.trim() && label.trim() !== (before.translations[locale] ?? '').trim())
    .map(([locale, label]) => ({ locale, label: label.trim() }))
  const deletes = Object.keys(before.translations).filter((locale) => !(after.translations[locale] ?? '').trim())
  return { upserts, deletes }
}
