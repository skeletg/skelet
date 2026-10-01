import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed, clip, sameJson } from '@/shared/saving/diff'
import { richTextToPlainText, type RichTextValue } from '../LexicalRichTextEditor'

// A rich-text field's diff entry: the plain text when the words changed, a formatting-only note when just the markup did
export function richTextEntry(label: string, before: unknown, after: unknown): SectionDiffEntry | null {
  if (sameJson(before, after)) return null
  const b = richTextToPlainText(before as RichTextValue)
  const a = richTextToPlainText(after as RichTextValue)
  return b === a ? changed(label, 'previous formatting', 'updated formatting') : changed(label, clip(b), clip(a))
}
