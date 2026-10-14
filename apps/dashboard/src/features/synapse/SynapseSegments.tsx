import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { RefreshCwIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/data-table/DataTable'
import { DataTableColumnHeader } from '@/components/data-table/DataTableColumnHeader'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import {
  type SynapseSegmentsQuery,
  SynapseOverviewDocument,
  SynapseRefreshSegmentDocument,
  SynapseSaveSegmentDocument,
  SynapseSegmentPreviewDocument,
  SynapseSegmentsDocument,
  SynapseSetSegmentLifecycleWindowsDocument,
  SynapseTrackedAttributesDocument,
} from './Synapse.gql'
import { SegmentTreeBuilder } from './SegmentTreeBuilder'
import { newGroup, nodeComplete, nodeToInput, summarizeNode, type SegNode, type SummaryNode } from './segment-tree'
import { formatNumber } from '@/lib/format'

type SegRow = SynapseSegmentsQuery['synapseSegments'][number]

export function SynapseSegments() {
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const [root, setRoot] = useState<SegNode>(() => newGroup('AND'))
  const [name, setName] = useState('')

  const { data: ov, error: ovError } = useQuery(SynapseOverviewDocument, { variables: { channelId, sinceDays: 30 }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const events = (ov?.synapseOverview.topEvents ?? []).map((e) => e.name)
  const { data: attrData, error: attrError } = useQuery(SynapseTrackedAttributesDocument, { variables: { channelId }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const attributes = attrData?.synapseTrackedAttributes ?? []

  const complete = nodeComplete(root)
  const { data: prev, loading: previewing, error: prevError } = useQuery(SynapseSegmentPreviewDocument, {
    variables: { channelId, root: nodeToInput(root) },
    skip: !isChannel || !complete,
    fetchPolicy: 'cache-and-network',
  })
  const previewCount = prev?.synapseSegmentPreview

  const { data: list, error: listError } = useQuery(SynapseSegmentsDocument, { variables: { channelId }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const [save, { loading: saving }] = useMutation(SynapseSaveSegmentDocument, { refetchQueries: [SynapseSegmentsDocument] })
  const [refresh, { loading: refreshing, error: refreshError }] = useMutation(SynapseRefreshSegmentDocument, { refetchQueries: [SynapseSegmentsDocument] })
  const error = ovError ?? attrError ?? prevError ?? listError ?? refreshError
  const [saveError, setSaveError] = useState<string | null>(null)

  const canSave = complete && name.trim().length > 0
  const onSave = async () => {
    if (!canSave) return
    setSaveError(null)
    try {
      await save({ variables: { channelId, name: name.trim(), root: nodeToInput(root) } })
      setName('')
      setRoot(newGroup('AND'))
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : String(e))
    }
  }

  const segColumns = useMemo<ColumnDef<SegRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Segment" />,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <Link to={`/synapse/segments/${row.original.id}`} className="truncate font-medium hover:underline">{row.original.name}</Link>
              {row.original.affinity?.auto ? (
                <Badge variant="secondary" className="text-[10px]" title="Auto-generated from a tracked attribute - managed automatically.">Auto</Badge>
              ) : null}
            </div>
            <div className="truncate text-xs text-muted-foreground">{row.original.slug}</div>
          </div>
        ),
      },
      {
        id: 'definition',
        header: 'Definition',
        enableSorting: false,
        cell: ({ row }) => <span className="text-xs text-muted-foreground">{summarizeNode((row.original.definition ?? null) as unknown as SummaryNode | null)}</span>,
      },
      {
        accessorKey: 'memberCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Members" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.memberCount.toLocaleString()}</span>,
      },
      {
        id: 'cadence',
        header: 'Cadence',
        enableSorting: false,
        cell: ({ row }) => <CadenceCell segmentId={row.original.id} windows={row.original.lifecycleWindows} />,
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) => (
          <button
            type="button"
            onClick={() => void refresh({ variables: { segmentId: row.original.id } }).catch(() => {})}
            disabled={refreshing}
            className="inline-flex items-center gap-1 rounded border px-2 py-1 text-xs text-muted-foreground hover:bg-muted disabled:opacity-50"
          >
            <RefreshCwIcon className="size-3" />
            Refresh
          </button>
        ),
      },
    ],
    [refresh, refreshing],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Segments"
        badges={isChannel ? <Badge variant="secondary">{selectedChannel?.name}</Badge> : undefined}
        meta={
          <span className="text-xs text-muted-foreground">
            Build a behavioural customer segment by combining events, traits, and attribute affinity with nested AND/OR
            groups. Saved segments are real customer segments - target them from Campaigns and Flows.
          </span>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load segments</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {!isChannel ? (
          <DetailPageSection>
            <Alert>
              <AlertTitle>Select a channel</AlertTitle>
              <AlertDescription>Segments are per channel - pick one in the header scope picker.</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <>
            <DetailPageSection
              title="Build a segment"
              description="Add conditions - an event (did / didn’t), a trait rule (RFM / lifecycle / engagement), or an attribute affinity - and combine them in nested AND/OR groups."
            >
              <SegmentTreeBuilder node={root} onChange={setRoot} events={events} attributes={attributes} />

              <div className="mt-4 flex flex-wrap items-center gap-3 border-t pt-3">
                <div className="text-sm">
                  {!complete ? (
                    <span className="text-muted-foreground">Complete every condition to preview.</span>
                  ) : previewing && previewCount === undefined ? (
                    <span className="text-muted-foreground">Counting…</span>
                  ) : (
                    <span>
                      <span className="text-lg font-semibold tabular-nums">{formatNumber(previewCount ?? 0)}</span>{' '}
                      <span className="text-muted-foreground">customers match</span>
                    </span>
                  )}
                </div>
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Segment name (e.g. Repeat buyers)"
                  className="ml-auto w-64 rounded border bg-background px-2.5 py-1.5 text-sm"
                />
                <button
                  type="button"
                  onClick={onSave}
                  disabled={!canSave || saving}
                  className="rounded bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                >
                  {saving ? 'Saving…' : 'Save segment'}
                </button>
              </div>
              {saveError ? <p className="mt-2 text-xs text-destructive">Couldn't save segment: {saveError}</p> : null}
            </DetailPageSection>

            <DetailPageSection title="Saved segments" description="Targetable from Campaigns and Flows. Click a segment for members, revenue & growth; Refresh re-evaluates membership.">
              <DataTable
                columns={segColumns}
                data={list?.synapseSegments ?? []}
                getRowId={(s) => s.id}
                searchPlaceholder="Search segments…"
                emptyState={<p className="py-6 text-sm text-muted-foreground">No behavioural segments yet - build one above.</p>}
              />
            </DetailPageSection>
          </>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// CadenceCell shows a segment's lifecycle-window override (or "Channel default")
// and lets an admin set/clear it inline - Members get these windows instead of the
// channel default (shortest wins across a customer's segments) - e.g. faster-wearing
// "All-terrain" buyers re-engaged sooner
type Windows = { newDays: number; activeDays: number; atRiskDays: number }
function CadenceCell({ segmentId, windows }: { segmentId: string; windows?: Windows | null }) {
  const [setWindows, { loading }] = useMutation(SynapseSetSegmentLifecycleWindowsDocument, { refetchQueries: [SynapseSegmentsDocument] })
  const [editing, setEditing] = useState(false)
  const [n, setN] = useState(windows?.newDays ?? 30)
  const [a, setA] = useState(windows?.activeDays ?? 90)
  const [r, setR] = useState(windows?.atRiskDays ?? 180)
  const [err, setErr] = useState<string | null>(null)

  if (!editing) {
    return (
      <div className="flex items-center gap-2">
        <span className={cn('text-xs', windows ? 'font-medium tabular-nums' : 'text-muted-foreground')}>
          {windows ? `${windows.newDays} / ${windows.activeDays} / ${windows.atRiskDays} d` : 'Channel default'}
        </span>
        <button
          type="button"
          onClick={() => { setN(windows?.newDays ?? 30); setA(windows?.activeDays ?? 90); setR(windows?.atRiskDays ?? 180); setErr(null); setEditing(true) }}
          className="rounded border px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted"
        >
          Edit
        </button>
      </div>
    )
  }
  const field = (v: number, set: (x: number) => void, label: string) => (
    <input
      type="number" min={1} value={v} aria-label={label}
      onChange={(e) => set(Math.max(1, Number(e.target.value) || 1))}
      className="w-14 rounded border bg-background px-1 py-0.5 text-xs tabular-nums"
    />
  )
  const onSet = async () => {
    if (!(n <= a && a <= r)) { setErr('Must be ascending: new ≤ active ≤ at-risk'); return }
    try {
      await setWindows({ variables: { segmentId, windows: { newDays: n, activeDays: a, atRiskDays: r } } })
      setEditing(false)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }
  const onClear = async () => {
    try {
      await setWindows({ variables: { segmentId, windows: null } })
      setEditing(false)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    }
  }
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1" title="new / active / at-risk (days since last order)">
        {field(n, setN, 'new days')}<span className="text-muted-foreground">/</span>
        {field(a, setA, 'active days')}<span className="text-muted-foreground">/</span>
        {field(r, setR, 'at-risk days')}
      </div>
      <div className="flex items-center gap-1">
        <button type="button" disabled={loading} onClick={onSet} className="rounded border px-1.5 py-0.5 text-[11px] hover:bg-muted disabled:opacity-50">Set</button>
        <button type="button" disabled={loading} onClick={onClear} className="rounded border px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted disabled:opacity-50">Clear</button>
        <button type="button" onClick={() => setEditing(false)} className="rounded px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted">Cancel</button>
      </div>
      {err ? <span className="text-[11px] text-destructive">{err}</span> : null}
    </div>
  )
}
