import type { ReactNode } from 'react'

// Marks matching runs, since the search also hits values, paths and option labels - without it a
// result row can look like it matched nothing
export function Highlight({ text, terms }: { text: string; terms: string[] }): ReactNode {
  if (terms.length === 0 || !text) return text
  const pattern = new RegExp(`(${terms.map(escapeRegExp).join('|')})`, 'gi')
  const pieces = text.split(pattern)
  if (pieces.length === 1) return text
  // Matches land at the odd indices - re-testing pieces with the /g regex flips as lastIndex moves
  return pieces.map((piece, i) =>
    i % 2 === 1 && piece ? (
      <mark
        key={i}
        className="rounded-[2px] bg-amber-200/70 px-px text-inherit dark:bg-amber-400/25"
      >
        {piece}
      </mark>
    ) : (
      piece
    ),
  )
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
