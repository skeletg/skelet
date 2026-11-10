import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

// The assistant writes light markdown - paragraphs, lists, bold, code and links - and the storefront renders that much,
// so a transcript reads the way the shopper saw it. A link stays text with its address beside it: what it pointed at is
// part of reviewing the answer, and a link a crafted question slipped in shouldn't be one click away in a staff tool

const TRAILING = /[.,;:!?'"]+$/
const INLINE = /\*\*([^*]+)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s)]+)/g
const BULLET = /^\s*[-*•]\s+/
const NUMBERED = /^\s*\d+[.)]\s+/
const HEADING = /^#{1,6}\s+/

function inline(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  for (const m of text.matchAll(INLINE)) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const k = `${key}-${m.index}`
    if (m[1] !== undefined) {
      out.push(
        <strong key={k} className="font-semibold">
          {m[1]}
        </strong>,
      )
    } else if (m[2] !== undefined) {
      out.push(
        <code key={k} className="rounded-sm bg-muted px-1 py-0.5 text-[0.9em]">
          {m[2]}
        </code>,
      )
    } else if (m[3] !== undefined) {
      out.push(
        <span key={k}>
          <span className="underline decoration-dotted underline-offset-2">{m[3]}</span>
          <span className="ml-1 break-all text-xs text-muted-foreground">{m[4]}</span>
        </span>,
      )
    } else {
      const bare = m[5].replace(TRAILING, '')
      out.push(
        <span key={k} className="break-all text-muted-foreground">
          {bare}
        </span>,
      )
      if (bare.length < m[5].length) out.push(m[5].slice(bare.length))
    }
    last = m.index + m[0].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

// One block's lines as paragraphs, headings and lists - a list usually follows a lead-in line in the same block
function block(lines: string[], key: string): ReactNode[] {
  const parts: ReactNode[] = []
  let i = 0
  while (i < lines.length) {
    const start = i
    const marker = BULLET.test(lines[i]) ? BULLET : NUMBERED.test(lines[i]) ? NUMBERED : null
    if (marker) {
      while (i < lines.length && marker.test(lines[i])) i++
      const items = lines.slice(start, i).map((l, j) => <li key={j}>{inline(l.replace(marker, ''), `${key}-${start + j}`)}</li>)
      parts.push(
        marker === BULLET ? (
          <ul key={`${key}-${start}`} className="list-disc space-y-1 pl-5">
            {items}
          </ul>
        ) : (
          <ol key={`${key}-${start}`} className="list-decimal space-y-1 pl-5">
            {items}
          </ol>
        ),
      )
      continue
    }
    if (HEADING.test(lines[i])) {
      parts.push(
        <p key={`${key}-${start}`} className="font-semibold">
          {inline(lines[i].replace(HEADING, ''), `${key}-${start}`)}
        </p>,
      )
      i++
      continue
    }
    while (i < lines.length && !BULLET.test(lines[i]) && !NUMBERED.test(lines[i]) && !HEADING.test(lines[i])) i++
    parts.push(
      <p key={`${key}-${start}`}>
        {lines.slice(start, i).map((l, j) => (
          <span key={j}>
            {j > 0 && <br />}
            {inline(l, `${key}-${start + j}`)}
          </span>
        ))}
      </p>,
    )
  }
  return parts
}

export function ReplyText({ text, className }: { text: string; className?: string }) {
  return (
    <div className={cn('space-y-2', className)}>
      {text
        .split(/\n{2,}/)
        .flatMap((b, i) => block(b.split('\n').filter((l) => l.trim() !== ''), String(i)))}
    </div>
  )
}
