import { z } from 'zod'
import { handleSlug } from '@/shared/schemas/fields'
import type { MenuItemTargetType } from '@/graphql/schema.types'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, fieldDiff, removed, sameSet } from '@/shared/saving/diff'

// Menu page - THE field list (docs/conventions.md, "Record forms"): the menu's slug, name and the
// channels serving it. Items are rows with their own dialog (below), not fields of the page

export const menuSchema = z
  .object({
    slug: handleSlug().describe('Handle the storefront fetches the menu by - lowercase letters, numbers, underscores and hyphens'),
    name: z.string().trim().min(1, 'Name is required').describe('Display name of the menu'),
    channelIds: z.array(z.string()).describe('IDs of the channels serving the menu'),
  })
  .describe('A navigation menu')

export type MenuValues = z.infer<typeof menuSchema>

export const MENU_LABELS: Record<keyof MenuValues, string> = { slug: 'Slug', name: 'Name', channelIds: 'Channels' }

export function toMenuValues(menu: { slug: string; name: string; channelIds?: string[] | null }): MenuValues {
  return { slug: menu.slug, name: menu.name, channelIds: menu.channelIds ?? [] }
}

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
    label: z.string().trim().min(1, 'Label is required').describe('Label shown in the menu'),
    kind: z.string().describe(`What the item links to, one of ${TARGET_KINDS.map((k) => k.value).join(', ')}`),
    url: z.string().describe('Link URL - required when kind is URL, ignored otherwise'),
    targetId: z.string().describe('UUID of the linked record - required for every kind but URL'),
    parentId: z.string().describe('ID of the parent item - empty string for a top-level item'),
    translations: z
      .record(z.string(), z.string())
      .describe('Translated labels keyed by locale code - an empty label removes that translation'),
  })
  .describe('A navigation menu item')
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
