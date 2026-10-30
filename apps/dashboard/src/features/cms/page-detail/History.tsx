import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { GitCompareIcon, RotateCcwIcon, XIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  CmsPageDocument,
  CmsPageRevisionsDocument,
  RestoreCmsPageRevisionDocument,
} from '../Cms.gql'
import {
  diffDocuments,
  diffFields,
  type BlockChange,
  type DiffSpan,
  type RevisionLike,
} from '../page-diff'

// Compact "2 hr ago"-style relative time, with the absolute timestamp left for
// the title attribute - Keeps the dense version list scannable
function relativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  const sec = Math.round(diffMs / 1000)
  if (sec < 45) return 'just now'
  const min = Math.round(sec / 60)
  if (min < 60) return `${min} min ago`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr} hr ago`
  const day = Math.round(hr / 24)
  if (day < 30) return `${day} day${day === 1 ? '' : 's'} ago`
  return new Date(iso).toLocaleDateString()
}

// What a version changed against the one saved before it, so rows that share a title can be told apart
function changeSummary(older: RevisionLike | undefined, rev: RevisionLike): string {
  if (!older) return 'Oldest kept version'
  const doc = diffDocuments(older.body, rev.body)
  const parts = diffFields(older, rev).map((f) => f.label)
  if (doc.changes.length > 0) {
    parts.push(doc.freeform ? 'Body' : `${doc.changes.length} block change${doc.changes.length === 1 ? '' : 's'}`)
  }
  return parts.length > 0 ? parts.join(', ') : 'No content changes'
}

// The page's saved versions for the active locale, newest first, with a compare against the current
// one and a restore. The backend keeps up to 50 per locale, so they reveal in pages
const REVISIONS_INITIAL = 10
const REVISIONS_STEP = 15

export function CmsPageHistory({
  pageId,
  locale,
  canManage,
  refetchVariables,
}: {
  pageId: string
  locale: string
  canManage: boolean
  refetchVariables: { id: string }
}) {
  const { data, loading } = useQuery(CmsPageRevisionsDocument, { variables: { pageId, locale } })
  const revisions = data?.cmsPageRevisions ?? []
  const [restore, { loading: restoring }] = useMutation(RestoreCmsPageRevisionDocument, {
    refetchQueries: [{ query: CmsPageDocument, variables: refetchVariables }, { query: CmsPageRevisionsDocument, variables: { pageId, locale } }],
  })
  const [shown, setShown] = useState(REVISIONS_INITIAL)
  const [compareId, setCompareId] = useState<string | null>(null)

  const visible = revisions.slice(0, shown)
  const remaining = revisions.length - visible.length
  const current = revisions[0]
  const compareBase = compareId ? revisions.find((r) => r.id === compareId) : undefined

  if (loading) return <p className="text-sm text-muted-foreground">Loading…</p>
  if (revisions.length === 0) {
    return (
      <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">
        No saved versions yet - one is kept each time the page is saved.
      </p>
    )
  }

  return (
    <div className="grid gap-3">
      <ol className="relative ml-[6px] border-l border-border pl-5">
        {visible.map((rev, i) => {
          const isCurrent = i === 0
          return (
            <li key={rev.id} className="relative pb-4 last:pb-0">
              <span
                className={cn(
                  'absolute -left-[27px] top-1 size-3 rounded-full border-2 border-background',
                  isCurrent ? 'bg-primary' : 'bg-muted-foreground/40',
                )}
              />
              <div className="flex items-start justify-between gap-3">
                <span className="min-w-0">
                  <span className="flex items-center gap-2">
                    <span className="text-sm font-medium text-foreground" title={new Date(rev.createdAt).toLocaleString()}>
                      {relativeTime(rev.createdAt)}
                    </span>
                    {isCurrent && (
                      <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
                        Current
                      </Badge>
                    )}
                  </span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">{changeSummary(revisions[i + 1], rev)}</span>
                </span>
                {!isCurrent && (
                  <span className="flex shrink-0 items-center gap-1">
                    <Button
                      variant={compareId === rev.id ? 'secondary' : 'ghost'}
                      size="sm"
                      className="h-8 gap-1.5 px-2 text-xs"
                      title="Compare this version with the current one"
                      onClick={() => setCompareId((id) => (id === rev.id ? null : rev.id))}
                    >
                      <GitCompareIcon className="size-3.5" />
                      Compare
                    </Button>
                    {canManage && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 gap-1.5 px-2 text-xs"
                        disabled={restoring}
                        title="Restore this version (saved as a new version, so it's reversible)"
                        onClick={() => restore({ variables: { id: rev.id } })}
                      >
                        <RotateCcwIcon className="size-3.5" />
                        Restore
                      </Button>
                    )}
                  </span>
                )}
              </div>
              {compareBase?.id === rev.id && current && (
                <div className="mt-2">
                  <RevisionComparePanel
                    base={compareBase}
                    target={current}
                    baseWhen={relativeTime(compareBase.createdAt)}
                    onClose={() => setCompareId(null)}
                  />
                </div>
              )}
            </li>
          )
        })}
      </ol>

      {remaining > 0 && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span>
            Showing {visible.length} of {revisions.length}
          </span>
          <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={() => setShown((n) => n + REVISIONS_STEP)}>
            Load {Math.min(remaining, REVISIONS_STEP)} more
          </Button>
        </div>
      )}
    </div>
  )
}

// DiffInline renders word-diff spans with add (green) / del (red strikethrough)
// highlighting
function DiffInline({ spans }: { spans: DiffSpan[] }) {
  if (spans.length === 0) return <span className="text-muted-foreground">(empty)</span>
  return (
    <span className="break-words">
      {spans.map((s, i) =>
        s.kind === 'eq' ? (
          <span key={i}>{s.text}</span>
        ) : s.kind === 'add' ? (
          <span key={i} className="rounded bg-success-soft text-success">
            {s.text}
          </span>
        ) : (
          <span key={i} className="rounded bg-destructive-soft text-destructive line-through">
            {s.text}
          </span>
        ),
      )}
    </span>
  )
}

function blockChangeBadge(kind: BlockChange['kind']): { label: string; cls: string } {
  switch (kind) {
    case 'added':
      return { label: 'Added', cls: 'bg-success-soft text-success' }
    case 'removed':
      return { label: 'Removed', cls: 'bg-destructive-soft text-destructive' }
    case 'reordered':
      return { label: 'Moved', cls: 'bg-warning-soft text-warning' }
    default:
      return { label: 'Changed', cls: 'bg-info-soft text-info' }
  }
}

// RevisionComparePanel shows what changed from a past revision to the current
// one: field-level word diffs + a block-level layout diff
function RevisionComparePanel({
  base,
  target,
  baseWhen,
  onClose,
}: {
  base: RevisionLike
  target: RevisionLike
  baseWhen: string
  onClose: () => void
}) {
  const fields = diffFields(base, target)
  const doc = diffDocuments(base.body, target.body)
  const nothing = fields.length === 0 && doc.changes.length === 0
  return (
    <div className="grid gap-3 rounded-md border bg-muted/30 p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-2 text-sm font-medium">
          <GitCompareIcon className="size-4 text-muted-foreground" />
          Changes from {baseWhen} → current
        </span>
        <Button variant="ghost" size="sm" className="h-7 px-2" onClick={onClose}>
          <XIcon className="size-4" />
        </Button>
      </div>
      {nothing ? (
        <p className="text-sm text-muted-foreground">No differences - these versions are identical.</p>
      ) : (
        <>
          {fields.length > 0 && (
            <div className="grid gap-2">
              {fields.map((f) => (
                <div key={f.key} className="grid gap-0.5">
                  <span className="text-xs font-medium text-muted-foreground">{f.label}</span>
                  <div className="text-sm leading-relaxed">
                    <DiffInline spans={f.spans} />
                  </div>
                </div>
              ))}
            </div>
          )}
          {doc.changes.length > 0 && (
            <div className="grid gap-2">
              <span className="text-xs font-medium text-muted-foreground">Layout</span>
              {doc.freeform ? (
                <p className="text-sm text-muted-foreground">Body content changed (not a block layout).</p>
              ) : (
                <ul className="grid gap-2">
                  {doc.changes.map((c) => {
                    const badge = blockChangeBadge(c.kind)
                    return (
                      <li key={c.id || c.label} className="rounded border bg-background px-2.5 py-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className={cn('rounded px-1.5 py-0.5 text-[10px] font-medium', badge.cls)}>
                            {badge.label}
                          </span>
                          <span className="text-sm font-medium">{c.label}</span>
                          {c.kind === 'reordered' && (
                            <span className="text-xs text-muted-foreground">
                              position {c.from + 1} → {c.to + 1}
                            </span>
                          )}
                        </div>
                        {c.kind === 'modified' && (
                          <div className="mt-1.5 grid gap-1 pl-1">
                            {c.childCount && (
                              <span className="text-xs text-muted-foreground">
                                Nested blocks: {c.childCount.from} → {c.childCount.to}
                              </span>
                            )}
                            {c.props.map((p) => (
                              <div key={p.key} className="grid gap-0.5">
                                <span className="text-[11px] text-muted-foreground">{p.label}</span>
                                <div className="text-xs leading-relaxed">
                                  <DiffInline spans={p.spans} />
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
