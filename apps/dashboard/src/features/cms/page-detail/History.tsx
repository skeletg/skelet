import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { toast } from 'sonner'
import { GitCompareIcon, HistoryIcon, RotateCcwIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatDateTime } from '@/lib/format'
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

// The page's saved versions for the active locale, newest first: the list on the left, the picked version's
// changes and its restore on the right. The backend keeps up to 50 per locale, so the list reveals in pages
const REVISIONS_INITIAL = 15
const REVISIONS_STEP = 20

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
  const revisions = useMemo(() => data?.cmsPageRevisions ?? [], [data])
  const [restore, { loading: restoring }] = useMutation(RestoreCmsPageRevisionDocument, {
    refetchQueries: [{ query: CmsPageDocument, variables: refetchVariables }, { query: CmsPageRevisionsDocument, variables: { pageId, locale } }],
  })
  const [shown, setShown] = useState(REVISIONS_INITIAL)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  // A layout diff word-diffs every changed block prop - worth doing once per load, not on every click
  const summaries = useMemo(() => revisions.map((rev, i) => changeSummary(revisions[i + 1], rev)), [revisions])

  if (loading && revisions.length === 0) return <p className="p-5 text-sm text-muted-foreground">Loading…</p>
  if (revisions.length === 0) {
    return (
      <div className="grid flex-1 place-items-center p-8 text-center">
        <div className="grid justify-items-center gap-2">
          <HistoryIcon className="size-8 text-muted-foreground/60" />
          <p className="text-sm font-medium">No saved versions yet</p>
          <p className="text-sm text-muted-foreground">One is kept each time the page is saved.</p>
        </div>
      </div>
    )
  }

  const visible = revisions.slice(0, shown)
  const remaining = revisions.length - visible.length
  const current = revisions[0]
  const selectedIndex = Math.max(0, revisions.findIndex((r) => r.id === selectedId))
  const selected = revisions[selectedIndex]
  const isCurrent = selectedIndex === 0

  async function onRestore() {
    try {
      await restore({ variables: { id: selected.id } })
      setSelectedId(null)
      toast.success('Version restored - it is now the current one.')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Restore failed.')
    }
  }

  return (
    <div className="grid min-h-0 flex-1 grid-rows-[minmax(0,14rem)_minmax(0,1fr)] md:grid-cols-[17rem_minmax(0,1fr)] md:grid-rows-1">
      <nav className="min-h-0 overflow-y-auto border-b md:border-b-0 md:border-r" aria-label="Versions">
        <ol className="divide-y">
          {visible.map((rev, i) => {
            const active = rev.id === selected.id
            return (
              <li key={rev.id}>
                <button
                  type="button"
                  aria-current={active || undefined}
                  onClick={() => setSelectedId(rev.id)}
                  className={cn(
                    'relative grid w-full gap-0.5 px-4 py-3 text-left transition-colors hover:bg-muted/50',
                    active && 'bg-muted hover:bg-muted',
                  )}
                >
                  {active && <span className="absolute inset-y-0 left-0 w-0.5 bg-primary" aria-hidden />}
                  <span className="flex items-center gap-2 text-sm font-medium">
                    <span title={formatDateTime(rev.createdAt)}>{relativeTime(rev.createdAt)}</span>
                    {i === 0 && (
                      <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
                        Current
                      </Badge>
                    )}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">{summaries[i]}</span>
                </button>
              </li>
            )
          })}
        </ol>
        {remaining > 0 && (
          <div className="border-t p-2">
            <Button variant="ghost" size="sm" className="w-full text-xs" onClick={() => setShown((n) => n + REVISIONS_STEP)}>
              Load {Math.min(remaining, REVISIONS_STEP)} more
            </Button>
          </div>
        )}
      </nav>

      <section className="grid min-h-0 content-start gap-4 overflow-y-auto p-5" aria-label="Selected version">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="grid gap-0.5">
            <h3 className="text-base font-semibold">{isCurrent ? 'Current version' : formatDateTime(selected.createdAt)}</h3>
            <p className="text-sm text-muted-foreground">
              {isCurrent
                ? `Saved ${relativeTime(selected.createdAt)} - what the page holds now.`
                : `Saved ${relativeTime(selected.createdAt)}. Restoring it undoes the changes below.`}
            </p>
          </div>
          {!isCurrent && canManage && (
            <Button type="button" size="sm" disabled={restoring} onClick={() => void onRestore()}>
              <RotateCcwIcon className="size-4" />
              {restoring ? 'Restoring…' : 'Restore this version'}
            </Button>
          )}
        </div>
        {isCurrent ? (
          revisions[1] ? (
            <RevisionDiff title="What this save changed" base={revisions[1]} target={current} />
          ) : (
            <p className="text-sm text-muted-foreground">The oldest version kept - there's nothing earlier to compare it with.</p>
          )
        ) : (
          <RevisionDiff title="Changed since this version" base={selected} target={current} />
        )}
      </section>
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

// RevisionDiff shows what changed from one revision to another: field-level word diffs + a block-level layout diff
function RevisionDiff({ title, base, target }: { title: string; base: RevisionLike; target: RevisionLike }) {
  const fields = diffFields(base, target)
  const doc = diffDocuments(base.body, target.body)
  const nothing = fields.length === 0 && doc.changes.length === 0
  return (
    <div className="grid gap-3">
      <span className="flex items-center gap-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        <GitCompareIcon className="size-3.5" />
        {title}
      </span>
      {nothing ? (
        <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">No differences - the content is identical.</p>
      ) : (
        <>
          {fields.length > 0 && (
            <ul className="divide-y rounded-md border">
              {fields.map((f) => (
                <li key={f.key} className="grid gap-1 px-3 py-2.5">
                  <span className="text-xs font-medium text-muted-foreground">{f.label}</span>
                  <div className="text-sm leading-relaxed">
                    <DiffInline spans={f.spans} />
                  </div>
                </li>
              ))}
            </ul>
          )}
          {doc.changes.length > 0 && (
            <div className="grid gap-2">
              <span className="text-xs font-medium text-muted-foreground">Layout</span>
              {doc.freeform ? (
                <p className="text-sm text-muted-foreground">Body content changed (not a block layout).</p>
              ) : (
                <ul className="divide-y rounded-md border">
                  {doc.changes.map((c) => {
                    const badge = blockChangeBadge(c.kind)
                    return (
                      <li key={c.id || c.label} className="px-3 py-2.5">
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
