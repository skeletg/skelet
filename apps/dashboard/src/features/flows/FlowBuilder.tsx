import { useCallback, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useUnsavedFields } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  addEdge,
  Background,
  Controls,
  ReactFlow,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { useConfigScope } from '@/lib/config-scope'
import { LibraryEmailsDocument } from '@/features/emails/Emails.gql'
import { TriggerNode, EmailNode, SMSNode, WaitNode, SplitNode, SegmentNode, WebhookNode } from './FlowNodes'
import { ConditionEditor } from './ConditionEditor'
import { Switch } from '@/components/ui/switch'
import {
  ADDABLE_NODES,
  NODE_LABELS,
  TRIGGER_LABELS,
  TRANSACTIONAL_TRIGGERS,
  defaultCondition,
  defaultNodeData,
  type FlowCondition,
  type FlowNodeData,
} from './flowTypes'
import { CreateFlowDocument, FlowDocument, FlowSegmentsDocument, FlowTriggerTypesDocument, SetFlowStatusDocument, UpdateFlowDocument } from './Flows.gql'
import { usePermissions } from '@/shared/auth/usePermissions'

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  draft: 'outline',
  active: 'default',
  paused: 'secondary',
  archived: 'outline',
}

type LoadedFlow = {
  id: string | null
  name: string
  triggerType: string
  status: string
  channelId: string
  nodes: Node[]
  edges: Edge[]
  enrollmentFilter: FlowCondition | null
  transactional: boolean
}

const genId = () => (globalThis.crypto?.randomUUID?.() ?? `n${Math.random().toString(36).slice(2)}`)

export function FlowBuilder() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const isNew = !id || id === 'new'

  const { data, loading } = useQuery(FlowDocument, { variables: { id: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })

  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="New flow" backTo="/flows" />
        <div className="p-6">
          <Alert>
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>Select a channel in the header before building a flow.</AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }
  if (!isNew && !data?.flow) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Flow" backTo="/flows" />
        <div className="p-6">{loading ? <LoadingState /> : null}</div>
      </DetailPageLayout>
    )
  }

  const initial: LoadedFlow = isNew
    ? {
        id: null,
        name: '',
        triggerType: 'order_placed',
        status: 'draft',
        channelId: effectiveChannelId ?? '',
        nodes: [{ id: genId(), type: 'trigger', position: { x: 240, y: 24 }, data: { label: 'order_placed' } }],
        edges: [],
        enrollmentFilter: null,
        transactional: TRANSACTIONAL_TRIGGERS.has('order_placed'),
      }
    : (() => {
        const f = data!.flow!
        const g = (f.graph as { nodes?: Node[]; edges?: Edge[] } | null) ?? { nodes: [], edges: [] }
        const tc = (f.triggerConfig as { enrollmentFilter?: FlowCondition; transactional?: boolean } | null) ?? {}
        return {
          id: f.id,
          name: f.name,
          triggerType: f.triggerType,
          status: f.status,
          channelId: f.channelId,
          nodes: (g.nodes ?? []).map((n) => ({ ...n, data: { ...(n.data as object) } })) as Node[],
          edges: (g.edges ?? []) as Edge[],
          enrollmentFilter: tc.enrollmentFilter ?? null,
          transactional: tc.transactional ?? false,
        }
      })()

  return <Builder key={initial.id ?? 'new'} initial={initial} onCreated={(nid) => void duringCommit(() => navigate(`/flows/${nid}`))} />
}

function Builder({ initial, onCreated }: { initial: LoadedFlow; onCreated: (id: string) => void }) {
  const [name, setName] = useState(initial.name)
  const [triggerType, setTriggerType] = useState(initial.triggerType)
  const [enrollmentFilter, setEnrollmentFilter] = useState<FlowCondition | null>(initial.enrollmentFilter)
  const [transactional, setTransactional] = useState(initial.transactional)
  const [nodes, setNodes, onNodesChange] = useNodesState(initial.nodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges)
  const [selId, setSelId] = useState<string | null>(null)
  const [flowId, setFlowId] = useState<string | null>(initial.id)
  const [status, setStatus] = useState(initial.status)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const nodeTypes = useMemo(
    () => ({ trigger: TriggerNode, send_email: EmailNode, send_sms: SMSNode, wait: WaitNode, split: SplitNode, add_segment: SegmentNode, webhook: WebhookNode }),
    [],
  )

  const { data: trigData } = useQuery(FlowTriggerTypesDocument)
  const { data: segData } = useQuery(FlowSegmentsDocument)
  const { data: emailsData } = useQuery(LibraryEmailsDocument, { variables: { channelId: initial.channelId }, skip: !initial.channelId })
  const segments = (segData?.customerSegments.items ?? []).map((s) => ({ id: s.id, name: s.name }))
  const libraryEmails = (emailsData?.libraryEmails ?? []).map((e) => ({ id: e.id, name: e.name }))

  const onConnect = useCallback((c: Connection) => setEdges((eds) => addEdge(c, eds)), [setEdges])

  function addNode(type: string) {
    const id = genId()
    setNodes((nds) => [
      ...nds,
      { id, type, position: { x: 200 + Math.random() * 120, y: 140 + nds.length * 90 }, data: defaultNodeData(type) as Record<string, unknown> },
    ])
    setSelId(id)
  }

  const selected = nodes.find((n) => n.id === selId) ?? null

  const patchData = useCallback(
    (patch: Partial<FlowNodeData>) => {
      if (!selId) return
      setNodes((nds) => nds.map((n) => (n.id === selId ? { ...n, data: { ...n.data, ...patch } } : n)))
    },
    [selId, setNodes],
  )

  function deleteSelected() {
    if (!selected || selected.type === 'trigger') return
    const rid = selected.id
    setNodes((nds) => nds.filter((n) => n.id !== rid))
    setEdges((eds) => eds.filter((e) => e.source !== rid && e.target !== rid))
    setSelId(null)
  }

  function onChangeTrigger(t: string) {
    setTriggerType(t)
    // keep the trigger node's label in sync for display
    setNodes((nds) => nds.map((n) => (n.type === 'trigger' ? { ...n, data: { ...n.data, label: t } } : n)))
  }

  const [createFlow] = useMutation(CreateFlowDocument)
  const [updateFlow] = useMutation(UpdateFlowDocument)
  const [setFlowStatus] = useMutation(SetFlowStatusDocument)

  function buildGraph() {
    return {
      nodes: nodes.map((n) => ({ id: n.id, type: n.type, position: n.position, data: n.data })),
      edges: edges.map((e) => ({ id: e.id, source: e.source, target: e.target, sourceHandle: e.sourceHandle ?? '' })),
    }
  }

  // xyflow writes selection and drag flags onto its own objects, so compare only what a save sends
  useUnsavedFields([
    name,
    triggerType,
    enrollmentFilter,
    transactional,
    status,
    nodes.map((n) => ({ id: n.id, type: n.type, position: n.position, data: n.data })),
    edges.map((e) => ({ id: e.id, source: e.source, target: e.target, type: e.type, data: e.data })),
  ])

  async function save(): Promise<string | null> {
    setError(null)
    setSaving(true)
    try {
      const triggerConfig: { enrollmentFilter?: FlowCondition; transactional?: boolean } = {}
      if (enrollmentFilter) triggerConfig.enrollmentFilter = enrollmentFilter
      if (transactional) triggerConfig.transactional = true
      const input = { channelId: initial.channelId, name: name.trim() || 'Untitled flow', triggerType, triggerConfig, graph: buildGraph() }
      if (flowId) {
        await updateFlow({ variables: { id: flowId, input } })
        return flowId
      }
      const res = await createFlow({ variables: { input } })
      const nid = res.data?.createFlow?.id ?? null
      if (nid) setFlowId(nid)
      return nid
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
      return null
    } finally {
      setSaving(false)
    }
  }

  async function onSaveClick() {
    const nid = await save()
    if (nid && !initial.id) onCreated(nid)
  }

  async function toggleStatus(next: string) {
    const nid = await save()
    if (!nid) return
    try {
      const res = await setFlowStatus({ variables: { id: nid, status: next } })
      setStatus(res.data?.setFlowStatus?.status ?? next)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Status change failed.')
    }
  }

  return (
    <DetailPageLayout
      leftSidebar={
        <aside className="flex w-48 shrink-0 flex-col border-r bg-card">
          <div className="border-b px-4 py-3">
            <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Add step</h2>
          </div>
          <div className="grid gap-1.5 p-3">
            {ADDABLE_NODES.map((t) => (
              <Button key={t} type="button" variant="outline" size="sm" className="justify-start" onClick={() => addNode(t)}>
                {NODE_LABELS[t]}
              </Button>
            ))}
          </div>
          <p className="mt-auto p-3 text-[11px] text-muted-foreground">
            Drag from a node's dot to another node to connect. A split has “yes”/“no” outputs.
          </p>
        </aside>
      }
      rightSidebar={
        <aside className="flex w-80 shrink-0 flex-col border-l bg-card">
          <div className="border-b px-4 py-3">
            <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {selected ? NODE_LABELS[selected.type ?? ''] ?? 'Step' : 'Flow settings'}
            </h2>
          </div>
          <div className="flex-1 overflow-y-auto p-4">
            <ConfigPanel
              selected={selected}
              name={name}
              onName={setName}
              triggerType={triggerType}
              onTrigger={onChangeTrigger}
              triggerTypes={trigData?.flowTriggerTypes ?? []}
              patch={patchData}
              onDelete={deleteSelected}
              segments={segments}
              libraryEmails={libraryEmails}
              enrollmentFilter={enrollmentFilter}
              onEnrollmentFilter={setEnrollmentFilter}
              transactional={transactional}
              onTransactional={setTransactional}
            />
          </div>
          {/* Pinned action bar (replaces the old top header) */}
          <div className="shrink-0 space-y-2 border-t p-3">
            {error && (
              <Alert variant="destructive">
                <AlertTitle>Error</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{name.trim() || (initial.id ? 'Flow' : 'New flow')}</span>
              <Badge variant={STATUS_VARIANT[status] ?? 'outline'} className="capitalize">
                {status}
              </Badge>
            </div>
            <div className="flex gap-2">
              <Button type="button" variant="outline" size="sm" className="flex-1" disabled={saving} onClick={onSaveClick}>
                {saving ? 'Saving…' : 'Save'}
              </Button>
              {status === 'active' ? (
                <Button type="button" variant="outline" size="sm" className="flex-1" onClick={() => toggleStatus('paused')}>
                  Pause
                </Button>
              ) : (
                <Button type="button" size="sm" className="flex-1" onClick={() => toggleStatus('active')}>
                  Activate
                </Button>
              )}
            </div>
          </div>
        </aside>
      }
    >
      <div className="h-full min-h-0 w-full">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          nodeTypes={nodeTypes}
          onNodeClick={(_, n) => setSelId(n.id)}
          onPaneClick={() => setSelId(null)}
          fitView
          proOptions={{ hideAttribution: true }}
        >
          <Background />
          <Controls />
        </ReactFlow>
      </div>
    </DetailPageLayout>
  )
}

function ConfigPanel({
  selected,
  name,
  onName,
  triggerType,
  onTrigger,
  triggerTypes,
  patch,
  onDelete,
  segments,
  libraryEmails,
  enrollmentFilter,
  onEnrollmentFilter,
  transactional,
  onTransactional,
}: {
  selected: Node | null
  name: string
  onName: (s: string) => void
  triggerType: string
  onTrigger: (t: string) => void
  triggerTypes: string[]
  patch: (p: Partial<FlowNodeData>) => void
  onDelete: () => void
  segments: Array<{ id: string; name: string }>
  libraryEmails: Array<{ id: string; name: string }>
  enrollmentFilter: FlowCondition | null
  onEnrollmentFilter: (c: FlowCondition | null) => void
  transactional: boolean
  onTransactional: (v: boolean) => void
}) {
  // A new email needs emails.manage - editing a campaign or flow doesn't bring it
  const canCreateEmail = usePermissions().has('emails.manage')
  if (!selected) {
    return (
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="flow-name">Flow name</Label>
          <Input id="flow-name" value={name} onChange={(e) => onName(e.target.value)} placeholder="Welcome series" />
        </div>
        <p className="text-xs text-muted-foreground">Click a step on the canvas to configure it, or add steps from the left.</p>
      </div>
    )
  }

  const d = selected.data as FlowNodeData

  if (selected.type === 'trigger') {
    return (
      <div className="grid gap-3">
        <div className="grid gap-2">
          <Label className="text-xs">When this happens…</Label>
          <Select value={triggerType} onValueChange={onTrigger}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {triggerTypes.map((t) => (
                <SelectItem key={t} value={t}>
                  {TRIGGER_LABELS[t] ?? t}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">The customer enters the flow when this event fires.</p>
        </div>
        <div className="grid gap-2 border-t pt-3">
          <label className="flex items-center gap-2 text-xs font-medium">
            <Switch
              checked={enrollmentFilter != null}
              onCheckedChange={(on) => onEnrollmentFilter(on ? defaultCondition('lifecycle') : null)}
            />
            Only enroll when…
          </label>
          {enrollmentFilter != null && (
            <>
              <ConditionEditor value={enrollmentFilter} onChange={onEnrollmentFilter} segments={segments} />
              <p className="text-xs text-muted-foreground">
                Customers who don’t match are skipped at enrollment (e.g. only marketing-opted-in, or only active buyers).
              </p>
            </>
          )}
        </div>
        <div className="grid gap-2 border-t pt-3">
          <label className="flex items-center gap-2 text-xs font-medium">
            <Switch checked={transactional} onCheckedChange={onTransactional} />
            Transactional
          </label>
          <p className="text-xs text-muted-foreground">
            Transactional flows (order receipts, account emails) send regardless of marketing consent, and their links aren’t click-wrapped.
            {TRANSACTIONAL_TRIGGERS.has(triggerType) ? ' Recommended for this trigger.' : ' Leave off for marketing flows.'}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="grid gap-4">
      {selected.type === 'send_email' && (
        <div className="grid gap-1.5">
          <Label className="text-xs">Email to send</Label>
          <Select value={d.emailId || undefined} onValueChange={(v) => patch({ emailId: v })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Pick an email…" />
            </SelectTrigger>
            <SelectContent>
              {libraryEmails.length === 0 ? (
                <SelectItem value="__none" disabled>
                  No emails yet - create one in the library
                </SelectItem>
              ) : (
                libraryEmails.map((e) => (
                  <SelectItem key={e.id} value={e.id}>
                    {e.name}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
          <div className="flex gap-3 text-xs">
            {d.emailId && (
              <Link to={`/emails/${d.emailId}`} className="text-primary hover:underline" target="_blank">
                Edit this email
              </Link>
            )}
            {canCreateEmail && (
              <Link to="/emails/new" className="text-primary hover:underline" target="_blank">
                New email
              </Link>
            )}
          </div>
          <p className="text-xs text-muted-foreground">Build emails in Marketing → Emails, then pick one here.</p>
        </div>
      )}

      {selected.type === 'send_sms' && (
        <div className="grid gap-1.5">
          <Label className="text-xs">Message</Label>
          <textarea
            value={d.smsBody ?? ''}
            onChange={(e) => patch({ smsBody: e.target.value })}
            rows={4}
            maxLength={480}
            placeholder="Hi {{customer.firstName}}! …"
            className="w-full rounded-md border bg-background px-2 py-1.5 text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Plain text. Merge tags: <code className="text-[11px]">{'{{customer.firstName}}'}</code>,{' '}
            <code className="text-[11px]">{'{{cart.recoveryUrl}}'}</code>,{' '}
            <code className="text-[11px]">{'{{product.url}}'}</code>. Sent only to customers opted in to marketing
            SMS with a phone on file.
          </p>
        </div>
      )}

      {selected.type === 'wait' && (
        <WaitConfig seconds={d.delaySeconds ?? 86400} onChange={(s) => patch({ delaySeconds: s })} />
      )}

      {selected.type === 'split' && (
        <div className="grid gap-1.5">
          <Label className="text-xs">Branch when…</Label>
          <ConditionEditor
            value={d.condition ?? { kind: 'segment', segmentId: '' }}
            onChange={(c) => patch({ condition: c })}
            segments={segments}
          />
          <p className="text-xs text-muted-foreground">Customers who match take the “yes” branch, everyone else “no”.</p>
        </div>
      )}

      {selected.type === 'add_segment' && (
        <div className="grid gap-1.5">
          <Label className="text-xs">Add to segment</Label>
          <Select value={d.segmentId || undefined} onValueChange={(v) => patch({ segmentId: v })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Pick a segment…" />
            </SelectTrigger>
            <SelectContent>
              {segments.length === 0 ? (
                <SelectItem value="__none" disabled>
                  No segments yet
                </SelectItem>
              ) : (
                segments.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
        </div>
      )}

      {selected.type === 'webhook' && (
        <div className="grid gap-1.5">
          <Label className="text-xs">POST URL</Label>
          <Input value={d.url ?? ''} onChange={(e) => patch({ url: e.target.value })} placeholder="https://example.com/hook" />
          <p className="text-xs text-muted-foreground">Fire-and-forget. Receives {`{ flowId, customerId, context }`}. Private hosts are blocked.</p>
        </div>
      )}

      <Button type="button" variant="outline" size="sm" className="text-destructive" onClick={onDelete}>
        Delete step
      </Button>
    </div>
  )
}

function WaitConfig({ seconds, onChange }: { seconds: number; onChange: (s: number) => void }) {
  const unit = seconds % 86400 === 0 ? 'days' : seconds % 3600 === 0 ? 'hours' : 'minutes'
  const amount = unit === 'days' ? seconds / 86400 : unit === 'hours' ? seconds / 3600 : Math.round(seconds / 60)
  const toSeconds = (amt: number, u: string) => (u === 'days' ? amt * 86400 : u === 'hours' ? amt * 3600 : amt * 60)
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs">Wait for</Label>
      <div className="flex gap-2">
        <Input
          type="number"
          min={1}
          value={amount}
          onChange={(e) => onChange(toSeconds(Math.max(1, Number(e.target.value || 1)), unit))}
          className="w-24"
        />
        <Select value={unit} onValueChange={(u) => onChange(toSeconds(amount, u))}>
          <SelectTrigger className="flex-1">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="minutes">minutes</SelectItem>
            <SelectItem value="hours">hours</SelectItem>
            <SelectItem value="days">days</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
