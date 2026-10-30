import { type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { type CmsAttributeRowFragment, type CmsPageRowFragment } from '../Cms.gql'
import { type FieldValue } from '../DynamicFieldRenderer'

export type PageTypeAttr = { isRequired: boolean; attribute: CmsAttributeRowFragment }


// Compact display of a dynamic field value for the save-confirm diff - rich text and JSON blobs
// collapse to a previous/updated marker, which still tells the operator what changed
function formatFieldValue(v: FieldValue, side: 'before' | 'after'): string {
  if (v == null || v === '') return ''
  if (Array.isArray(v)) return v.map(String).join(', ')
  if (typeof v === 'object') return side === 'before' ? 'previous' : 'updated'
  return String(v)
}

export function dynamicFieldDiff(
  attrs: PageTypeAttr[],
  original: Record<string, FieldValue>,
  draft: Record<string, FieldValue>,
): SectionDiffEntry[] {
  const out: SectionDiffEntry[] = []
  for (const pta of attrs) {
    const key = pta.attribute.slug
    if (JSON.stringify(original[key] ?? null) === JSON.stringify(draft[key] ?? null)) continue
    out.push({
      label: pta.attribute.content?.name ?? key,
      before: formatFieldValue(original[key], 'before'),
      after: formatFieldValue(draft[key], 'after'),
    })
  }
  return out
}

export function workflowLabel(state: string): string {
  switch (state) {
    case 'DRAFT':
      return 'Draft'
    case 'IN_REVIEW':
      return 'In review'
    case 'APPROVED':
      return 'Approved'
    case 'PUBLISHED':
      return 'Published'
    default:
      return state
  }
}

export type PublishPage = Pick<CmsPageRowFragment, 'id' | 'isPublished' | 'scheduledAt' | 'workflowState' | 'assignedTo' | 'channelListings'>

const shortDate = (iso: string) => new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })

// Live on a channel takes both the page's own flag and that channel's listing, so the label reads the two together
export function publishStatus(page: PublishPage): { label: string; dot: string } {
  const live = page.channelListings.filter((l) => l.isPublished).length
  if (page.isPublished) {
    return live > 0
      ? { label: `Live on ${live} channel${live === 1 ? '' : 's'}`, dot: 'bg-success' }
      : { label: 'Published, on no channel', dot: 'bg-warning' }
  }
  if (page.scheduledAt) return { label: `Scheduled for ${shortDate(page.scheduledAt as string)}`, dot: 'bg-info' }
  return { label: 'Unpublished', dot: 'bg-muted-foreground/50' }
}
