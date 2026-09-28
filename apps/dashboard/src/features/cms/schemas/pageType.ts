import { z } from 'zod'
import { numberText } from '@/shared/schemas/fields'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'

// New CMS page type - a page template. The name and description are seeded for the locale being
// edited; attributes are assigned on the type's own page afterwards

export const newPageTypeSchema = z
  .object({
    slug: z.string().trim().min(1, 'Slug is required').describe('Stable identifier of the page type'),
    name: z.string().trim().min(1, 'Name is required').describe('Name in the edited locale'),
    description: z.string().describe('What the page type is for, in the edited locale - empty string for none'),
    icon: z.string().describe('Lucide icon name, such as FileText - empty string for none'),
  })
  .describe('A new CMS page type: a template pages are created from')

export type NewPageTypeValues = z.infer<typeof newPageTypeSchema>

export const emptyNewPageType: NewPageTypeValues = { slug: '', name: '', description: '', icon: '' }

export function toNewPageTypeInput(v: NewPageTypeValues, locale: string) {
  return {
    slug: v.slug.trim(),
    icon: v.icon.trim() || null,
    isSystem: false,
    position: 0,
    locale,
    name: v.name.trim(),
    description: v.description.trim() || null,
  }
}

// The page type's own page - its naming (per locale), URL structure, SEO defaults and starter
// template, one form behind the header Save. Attribute assignments stay explicit rows

// As the backend stores it (internal/cms validateSlugPath): spaces and outer slashes dropped
export const normalizeRoot = (v: string) => v.trim().replace(/^\/+|\/+$/g, '')

const ROOT = /^[a-z0-9._-]+(\/[a-z0-9._-]+)*$/
const validRoot = (v: string) => normalizeRoot(v) === '' || ROOT.test(normalizeRoot(v))
const ROOT_MESSAGE = 'Lowercase letters, digits, dots, dashes and underscores, segments split by /'

export const pageTypeSchema = z
  .object({
    name: z.string().describe('Name in the edited locale - required when a description is set'),
    description: z.string().describe('What the page type is for, in the edited locale - empty string for none'),
    position: numberText({ optional: true, int: true }).describe('Sort position among page types, lower first, as a whole number written as text'),
    icon: z.string().describe('Lucide icon name, such as FileText - empty string for none'),
    rootSlug: z
      .string()
      .refine(validRoot, ROOT_MESSAGE)
      .describe('URL prefix for pages of this type, such as blog/guides - empty string for the bare path'),
    channelRoots: z
      .record(z.string(), z.string())
      .describe('Per-channel URL prefix overrides, keyed by channel ID - empty string uses rootSlug'),
    isSingleton: z.boolean().describe('Only one page of this type may exist'),
    defaultMetaTitle: z
      .string()
      .describe('SEO title pattern, such as "{title} | Guides", used by pages of this type that leave theirs blank - empty string for none'),
    defaultSeoNoindex: z.boolean().describe('Noindex setting inherited by pages of this type that leave theirs unset'),
    // Opaque to the form - the block tree's recursive type would blow up TanStack's deep field paths
    blocks: z.array(z.unknown()).describe('Starter block layout new pages of this type begin with'),
  })
  .describe("A CMS page type's naming, URL structure, SEO defaults and starter layout")
  .superRefine((v, ctx) => {
    // The translation row needs a name - a description alone would be silently dropped
    if (v.name.trim() === '' && v.description.trim() !== '') {
      ctx.addIssue({ code: 'custom', path: ['name'], message: 'A description needs a name' })
    }
    for (const [channelId, root] of Object.entries(v.channelRoots)) {
      if (!validRoot(root)) ctx.addIssue({ code: 'custom', path: ['channelRoots', channelId], message: ROOT_MESSAGE })
    }
  })

export type PageTypeValues = z.infer<typeof pageTypeSchema>

export const PAGE_TYPE_LABELS: Record<keyof PageTypeValues, string> = {
  name: 'Name',
  description: 'Description',
  position: 'Position',
  icon: 'Icon',
  rootSlug: 'URL root',
  channelRoots: 'Per-channel roots',
  isSingleton: 'Singleton',
  defaultMetaTitle: 'Default meta title',
  defaultSeoNoindex: 'Default noindex',
  blocks: 'Starter blocks',
}

type ExistingPageType = {
  icon?: string | null
  position: number
  rootSlug?: string | null
  channelRoots: { channelId: string; rootSlug: string }[]
  isSingleton: boolean
  defaultMetaTitle?: string | null
  defaultSeoNoindex: boolean
  defaultBlocks?: unknown
  allContent: { locale: string; name: string; description?: string | null }[]
}

export function toPageTypeValues(type: ExistingPageType, locale: string, blocks: unknown[]): PageTypeValues {
  const content = type.allContent.find((c) => c.locale === locale)
  return {
    name: content?.name ?? '',
    description: content?.description ?? '',
    position: String(type.position),
    icon: type.icon ?? '',
    rootSlug: type.rootSlug ?? '',
    channelRoots: Object.fromEntries(type.channelRoots.map((r) => [r.channelId, r.rootSlug])),
    isSingleton: type.isSingleton,
    defaultMetaTitle: type.defaultMetaTitle ?? '',
    defaultSeoNoindex: type.defaultSeoNoindex,
    blocks,
  }
}

// The channels whose override changed - blank and missing both mean "no override"
export function changedChannelRoots(before: PageTypeValues, after: PageTypeValues): { channelId: string; rootSlug: string }[] {
  const ids = new Set([...Object.keys(before.channelRoots), ...Object.keys(after.channelRoots)])
  return [...ids]
    .map((channelId) => ({ channelId, was: normalizeRoot(before.channelRoots[channelId] ?? ''), rootSlug: normalizeRoot(after.channelRoots[channelId] ?? '') }))
    .filter((r) => r.was !== r.rootSlug)
    .map(({ channelId, rootSlug }) => ({ channelId, rootSlug }))
}

const blockTypes = (v: PageTypeValues) => (v.blocks as { type: string }[]).map((b) => b.type).join(', ')

// Which of the type's writes the edits need - each is its own mutation
export function pageTypeChanges(before: PageTypeValues, after: PageTypeValues) {
  return {
    display: before.icon.trim() !== after.icon.trim() || (Number(before.position) || 0) !== (Number(after.position) || 0),
    content: before.name.trim() !== after.name.trim() || before.description.trim() !== after.description.trim(),
    structure:
      normalizeRoot(before.rootSlug) !== normalizeRoot(after.rootSlug) ||
      before.isSingleton !== after.isSingleton ||
      before.defaultMetaTitle !== after.defaultMetaTitle ||
      before.defaultSeoNoindex !== after.defaultSeoNoindex,
    channelRoots: changedChannelRoots(before, after),
    blocks: JSON.stringify(before.blocks) !== JSON.stringify(after.blocks),
  }
}

export function pageTypeDiff(channelName: (id: string) => string) {
  return (original: PageTypeValues, draft: PageTypeValues): SectionDiffEntry[] => [
    ...fieldDiff(original, draft, [
      { label: PAGE_TYPE_LABELS.name, value: (v) => v.name },
      { label: PAGE_TYPE_LABELS.description, value: (v) => v.description },
      { label: PAGE_TYPE_LABELS.position, value: (v) => Number(v.position) || 0 },
      { label: PAGE_TYPE_LABELS.icon, value: (v) => v.icon },
      { label: PAGE_TYPE_LABELS.rootSlug, value: (v) => normalizeRoot(v.rootSlug) },
      { label: PAGE_TYPE_LABELS.isSingleton, value: (v) => v.isSingleton },
      { label: PAGE_TYPE_LABELS.defaultMetaTitle, value: (v) => v.defaultMetaTitle },
      { label: PAGE_TYPE_LABELS.defaultSeoNoindex, value: (v) => v.defaultSeoNoindex },
    ]),
    ...changedChannelRoots(original, draft)
      .map((r) => changed(`Root · ${channelName(r.channelId)}`, normalizeRoot(original.channelRoots[r.channelId] ?? ''), r.rootSlug))
      .filter((e) => e !== null),
    ...(JSON.stringify(original.blocks) !== JSON.stringify(draft.blocks)
      ? [changed(PAGE_TYPE_LABELS.blocks, blockTypes(original) || 'none', blockTypes(draft) || 'none')].filter((e) => e !== null)
      : []),
  ]
}
