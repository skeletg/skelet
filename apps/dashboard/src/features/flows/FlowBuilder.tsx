import { useMemo, useRef, useState, type DragEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useUnsavedFields } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  addEdge,
  Background,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { CircleCheckIcon, CircleHelpIcon, MaximizeIcon, NetworkIcon, Redo2Icon, Undo2Icon, ZoomInIcon, ZoomOutIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { PanelTabs, Rail, RailGroup, RailItem, SidePanel, ToolButton, ToolGroup, ToolPill, ToolRow } from '@/components/layout/workbench'
import { WORKBENCH } from '@/components/layout/workbench-styles'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { LoadingState } from '@/components/ui/loading-state'
import { useConfigScope } from '@/lib/config-scope'
import { LibraryEmailNamesDocument } from '@/features/emails/Emails.gql'
import { TriggerNode, EmailNode, SMSNode, WaitNode, SplitNode, SegmentNode, WebhookNode, StepIcon } from './FlowNodes'
import { FlowStart } from './FlowStart'
import { CheckMark, StepSettings } from './StepSettings'
import { FlowCanvasContext, type FlowCanvas } from './flowCanvasContext'
import { GAP_X, GAP_Y, STEP_WIDTH, attach, detach, flowChecks, tailOf, tidy, type Check, type Link, type Step } from './flowGraph'
import { FLOW_RECIPES, buildRecipe, type FlowRecipe } from './flowRecipes'
import { FLOW_SHORTCUTS, useFlowShortcuts } from './useFlowShortcuts'
import {
  ADDABLE_NODES,
  NODE_HINTS,
  NODE_LABELS,
  STEP_GROUPS,
  TRIGGER_LABELS,
  defaultNodeData,
  type FlowCondition,
  type FlowNodeData,
} from './flowTypes'
import { CreateFlowDocument, FlowDocument, FlowSegmentsDocument, FlowTriggersDocument, SetFlowStatusDocument, UpdateFlowDocument } from './Flows.gql'
import { LoadError } from '@/shared/errors/LoadError'
import { NotFound } from '@/shared/errors/NotFound'

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

  const { data, loading, error, refetch } = useQuery(FlowDocument, { variables: { id: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })

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
  // A failed load or a missing record used to leave the editor's header over an empty body
  if (!isNew && !data?.flow && error) {
    return <LoadError title="Couldn't load this flow" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }
  if (!isNew && !data?.flow && !loading) return <NotFound />
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
        transactional: true, // an order flow, like the trigger a new flow starts on
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

  return (
    <ReactFlowProvider key={initial.id ?? 'new'}>
      <Builder initial={initial} onCreated={(nid) => void duringCommit(() => navigate(`/flows/${nid}`))} />
    </ReactFlowProvider>
  )
}

type Tab = 'step' | 'flow'

type Snapshot = { nodes: Step[]; edges: Link[]; name: string; triggerType: string; enrollmentFilter: FlowCondition | null; transactional: boolean }

const STEP_DRAG = 'application/x-skelet-flow-step'
const HISTORY = 50
const SAME_EDIT_MS = 1200 // a pause shorter than this keeps typing in one undo step
const SETTLE_MS = 60 // xyflow measures new steps a frame or two after they're added, and fitting needs their size
const FIT = { padding: 0.3, maxZoom: 1 }

const STATUS_HELP: Record<string, string> = {
  draft: 'Not running. Activate it to start enrolling customers.',
  active: 'Running. Customers who trigger it are enrolled.',
  paused: 'Nobody new enters, and customers already in it wait.',
  archived: 'Not running.',
}

// xyflow keeps selection, measurements and drag flags on its own objects - these carry only what the flow is
const toStep = (n: Node): Step => ({ id: n.id, type: n.type, position: { x: n.position.x, y: n.position.y }, data: n.data })
const toLink = (e: Edge): Link => ({ id: e.id, source: e.source, target: e.target, sourceHandle: e.sourceHandle || null })
const toNode = (s: Step, selected = false): Node => ({ ...s, selected })
const toEdge = (l: Link): Edge => ({ id: l.id, source: l.source, target: l.target, ...(l.sourceHandle ? { sourceHandle: l.sourceHandle } : {}) })
const clock = () => Date.now()

const flagKey = (checks: Check[]) =>
  checks
    .filter((c) => c.step)
    .map((c) => `${c.level === 'error' ? 'e' : 'w'}${c.step}`)
    .join(' ')

function flagsOf(key: string): FlowCanvas['flags'] {
  const flags = new Map<string, 'error' | 'warning'>()
  for (const f of key ? key.split(' ') : []) {
    if (flags.get(f.slice(1)) !== 'error') flags.set(f.slice(1), f.startsWith('e') ? 'error' : 'warning')
  }
  return flags
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
  const { zoomIn, zoomOut, fitView, screenToFlowPosition } = useReactFlow()

  const [tab, setTab] = useState<Tab>('flow')
  const [lastSel, setLastSel] = useState<string | null>(null)
  if (selId !== lastSel) {
    setLastSel(selId)
    setTab(selId ? 'step' : 'flow')
  }

  const nodeTypes = useMemo(
    () => ({ trigger: TriggerNode, send_email: EmailNode, send_sms: SMSNode, wait: WaitNode, split: SplitNode, add_segment: SegmentNode, webhook: WebhookNode }),
    [],
  )

  const { data: trigData, error: trigError } = useQuery(FlowTriggersDocument)
  const { data: segData, error: segError } = useQuery(FlowSegmentsDocument)
  const { data: emailsData, error: emailsError } = useQuery(LibraryEmailNamesDocument, { variables: { channelId: initial.channelId }, skip: !initial.channelId })
  const triggers = trigData?.flowTriggers ?? []
  const trigger = triggers.find((t) => t.type === triggerType)
  const unloaded = [trigError && 'the triggers', segError && 'the segments', emailsError && 'the emails'].filter((x): x is string => !!x)
  const segments = (segData?.customerSegments.items ?? []).map((s) => ({ id: s.id, name: s.name }))
  const libraryEmails = emailsData?.libraryEmails ?? []
  const names = useMemo(
    () => ({
      emails: new Map((emailsData?.libraryEmails ?? []).map((e) => [e.id, e.name])),
      segments: new Map((segData?.customerSegments.items ?? []).map((g) => [g.id, g.name])),
    }),
    [emailsData, segData],
  )

  const steps = nodes.map(toStep)
  const links = edges.map(toLink)
  const selected = nodes.find((n) => n.id === selId) ?? null

  const checks = flowChecks(steps, links, {
    oneTimeLink: trigger?.oneTimeLink ?? false,
    emails: emailsData ? new Set(libraryEmails.map((e) => e.id)) : null,
    segments: segData ? new Set(segments.map((s) => s.id)) : null,
  })
  const errors = checks.filter((c) => c.level === 'error').length
  // Keyed on what is flagged rather than on the checks themselves, so dragging a step doesn't redraw every other one
  const flags = flagsOf(flagKey(checks))
  const canvas: FlowCanvas = { names, flags }

  const [past, setPast] = useState<Snapshot[]>([])
  const [future, setFuture] = useState<Snapshot[]>([])
  const lastMark = useRef({ key: '', at: 0 })
  const take = (): Snapshot => ({ nodes: steps, edges: links, name, triggerType, enrollmentFilter, transactional })

  // Called before a change. A run of the same kind of change (typing a name, nudging one step's settings) is one undo
  function checkpoint(key = '') {
    const now = clock()
    const again = key !== '' && lastMark.current.key === key && now - lastMark.current.at < SAME_EDIT_MS
    lastMark.current = { key, at: now }
    if (again) return
    const snap = take()
    setPast((p) => (p.length > 0 && JSON.stringify(p[p.length - 1]) === JSON.stringify(snap) ? p : [...p.slice(1 - HISTORY), snap]))
    setFuture([])
  }

  function restore(to: Snapshot) {
    lastMark.current = { key: '', at: 0 }
    setNodes(to.nodes.map((n) => toNode(n)))
    setEdges(to.edges.map(toEdge))
    setName(to.name)
    setTriggerType(to.triggerType)
    setEnrollmentFilter(to.enrollmentFilter)
    setTransactional(to.transactional)
    setSelId((id) => (id && to.nodes.some((n) => n.id === id) ? id : null))
  }

  function undo() {
    const now = JSON.stringify(take())
    const at = past.findLastIndex((s) => JSON.stringify(s) !== now)
    if (at === -1) return setPast([])
    setFuture((f) => [...f, take()])
    setPast(past.slice(0, at))
    restore(past[at]!)
  }

  function redo() {
    const to = future[future.length - 1]
    if (!to) return
    setPast((p) => [...p, take()])
    setFuture((f) => f.slice(0, -1))
    restore(to)
  }

  function selectStep(id: string | null) {
    setNodes((nds) => nds.map((n) => (n.selected === (n.id === id) ? n : { ...n, selected: n.id === id })))
    setSelId(id)
  }

  // A dot leads one way, as the engine reads it - connecting it again moves the connection
  function onConnect(c: Connection) {
    checkpoint()
    setEdges((eds) => addEdge(c, eds.filter((e) => !(e.source === c.source && (e.sourceHandle || null) === (c.sourceHandle || null)))))
  }

  function addNode(type: string, at?: { x: number; y: number }) {
    checkpoint()
    const id = genId()
    const data = defaultNodeData(type) as Record<string, unknown>
    const anchor = at ? null : ((selected && steps.find((s) => s.id === selected.id)) ?? tailOf(steps, links))
    const hung = anchor ? attach(steps, links, anchor.id, { id, type }, genId) : null
    const under = anchor?.position ?? { x: 240, y: 24 }
    const side = !hung?.attached ? GAP_X : anchor?.type === 'split' ? (hung.handle === 'no' ? GAP_X / 2 : -GAP_X / 2) : 0
    const position = at ?? { x: under.x + side, y: under.y + GAP_Y }
    const pushed = new Set(hung?.pushed ?? [])
    setNodes((nds) => [
      ...nds.map((n) => ({ ...n, selected: false, position: pushed.has(n.id) ? { x: n.position.x, y: n.position.y + GAP_Y } : n.position })),
      toNode({ id, type, position, data }, true),
    ])
    if (hung?.attached) setEdges(hung.links.map(toEdge))
    setSelId(id)
  }

  function onDropStep(e: DragEvent<HTMLDivElement>) {
    const type = e.dataTransfer.getData(STEP_DRAG)
    if (!(ADDABLE_NODES as readonly string[]).includes(type)) return
    e.preventDefault()
    const at = screenToFlowPosition({ x: e.clientX, y: e.clientY })
    addNode(type, { x: at.x - STEP_WIDTH / 2, y: at.y - GAP_Y / 5 }) // under the pointer, not hanging from its corner
  }

  function patchData(patch: Partial<FlowNodeData>) {
    if (!selId) return
    checkpoint(`step:${selId}`)
    setNodes((nds) => nds.map((n) => (n.id === selId ? { ...n, data: { ...n.data, ...patch } } : n)))
  }

  function deleteSelected() {
    if (!selected || selected.type === 'trigger') return
    checkpoint()
    const rid = selected.id
    setNodes((nds) => nds.filter((n) => n.id !== rid))
    setEdges(detach(steps, links, rid, genId).map(toEdge))
    setSelId(null)
  }

  function removeSelection() {
    const gone = new Set(nodes.filter((n) => n.selected && n.type !== 'trigger').map((n) => n.id))
    if (gone.size <= 1 && selected) return deleteSelected()
    if (gone.size === 0 && !edges.some((e) => e.selected)) return
    checkpoint()
    setNodes((nds) => nds.filter((n) => !gone.has(n.id)))
    setEdges((eds) => eds.filter((e) => !e.selected && !gone.has(e.source) && !gone.has(e.target)))
    setSelId(null)
  }

  function duplicateSelected() {
    if (!selected || selected.type === 'trigger') return
    checkpoint()
    const id = genId()
    const copy = toNode({ id, type: selected.type, position: { x: selected.position.x + 40, y: selected.position.y + 40 }, data: structuredClone(selected.data) }, true)
    setNodes((nds) => [...nds.map((n) => ({ ...n, selected: false })), copy])
    setSelId(id)
  }

  function tidyUp() {
    checkpoint()
    const at = tidy(steps, links)
    setNodes((nds) => nds.map((n) => ({ ...n, position: at.get(n.id) ?? n.position })))
    setTimeout(() => void fitView({ ...FIT, duration: 200 }), SETTLE_MS)
  }

  function startFrom(recipe: FlowRecipe) {
    const trigger = steps.find((s) => s.type === 'trigger')
    if (!trigger) return
    checkpoint()
    const built = buildRecipe(recipe, trigger, genId)
    setTriggerType(recipe.trigger)
    setTransactional(false)
    setEnrollmentFilter(null)
    if (!name.trim()) setName(recipe.name)
    setNodes([toNode({ ...trigger, data: { ...trigger.data, label: recipe.trigger } }), ...built.steps.map((n) => toNode(n))])
    setEdges(built.links.map(toEdge))
    setSelId(null)
    setTimeout(() => void fitView({ ...FIT, duration: 200 }), SETTLE_MS)
  }

  // A flow is transactional or not by what starts it, so a new trigger resets the designation to its own default
  function onChangeTrigger(t: string) {
    checkpoint()
    setTriggerType(t)
    setTransactional(triggers.find((x) => x.type === t)?.transactional ?? false)
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

  useFlowShortcuts({
    undo,
    redo,
    duplicate: duplicateSelected,
    remove: removeSelection,
    deselect: () => selectStep(null),
    save: () => void onSaveClick(),
  })

  const stepName = (id: string | null) => {
    const type = id ? nodes.find((n) => n.id === id)?.type : undefined
    return type ? (NODE_LABELS[type] ?? 'Step') : 'The flow'
  }
  // A recipe is a marketing flow, so it is only offered on a trigger the suite keeps to marketing
  const recipes = nodes.length === 1 && edges.length === 0 ? FLOW_RECIPES.filter((r) => triggers.some((t) => t.type === r.trigger && !t.transactional)) : []

  const actions = (
    <div className="space-y-2">
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex items-center gap-1">
        <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Undo" disabled={past.length === 0} onClick={undo}>
          <Undo2Icon className="size-4" />
        </Button>
        <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Redo" disabled={future.length === 0} onClick={redo}>
          <Redo2Icon className="size-4" />
        </Button>
        <Button type="button" variant="outline" size="sm" className="flex-1" disabled={saving} onClick={onSaveClick}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
        {status === 'active' ? (
          <Button type="button" variant="outline" size="sm" className="flex-1" onClick={() => toggleStatus('paused')}>
            Pause
          </Button>
        ) : (
          <Button
            type="button"
            size="sm"
            className="flex-1"
            disabled={errors > 0}
            title={errors > 0 ? `${errors === 1 ? 'One thing' : `${errors} things`} to fix first - see the Flow tab` : undefined}
            onClick={() => toggleStatus('active')}
          >
            Activate
          </Button>
        )}
      </div>
    </div>
  )

  return (
    <DetailPageLayout
      className={WORKBENCH}
      toc={false}
      leftSidebar={
        <Rail data-flow-rail label="Steps" title="Steps" hint="Click to add, or drag onto the canvas">
          {STEP_GROUPS.map((g, i) => (
            <RailGroup key={g.label} label={g.label} first={i === 0}>
              {g.steps.map((type) => (
                <RailItem
                  key={type}
                  label={NODE_LABELS[type] ?? type}
                  hint={NODE_HINTS[type] ?? ''}
                  icon={<StepIcon type={type} className="size-4" />}
                  aria-label={`Add ${NODE_LABELS[type] ?? type}`}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData(STEP_DRAG, type)
                    e.dataTransfer.effectAllowed = 'copy'
                  }}
                  onClick={() => addNode(type)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      addNode(type)
                    }
                  }}
                />
              ))}
            </RailGroup>
          ))}
        </Rail>
      }
      rightSidebar={
        <SidePanel footer={actions}>
          <PanelTabs
            value={tab}
            onChange={setTab}
            tabs={[
              {
                value: 'step' as Tab,
                label: 'Step',
                content: selected ? (
                  <StepSettings
                    key={selected.id}
                    selected={selected}
                    found={checks.filter((c) => c.step === selected.id)}
                    channelId={initial.channelId}
                    triggerType={triggerType}
                    onTrigger={onChangeTrigger}
                    triggers={triggers}
                    patch={patchData}
                    onDelete={deleteSelected}
                    onDuplicate={duplicateSelected}
                    segments={segments}
                    libraryEmails={libraryEmails}
                    enrollmentFilter={enrollmentFilter}
                    onEnrollmentFilter={(c) => {
                      checkpoint('filter')
                      setEnrollmentFilter(c)
                    }}
                    transactional={transactional}
                    onTransactional={(v) => {
                      checkpoint()
                      setTransactional(v)
                    }}
                  />
                ) : (
                  <p className="text-sm text-muted-foreground">Select a step on the canvas to edit it.</p>
                ),
              },
              {
                value: 'flow' as Tab,
                label: 'Flow',
                content: (
                  <>
                    {unloaded.length > 0 ? (
                      <Alert variant="destructive">
                        <AlertDescription>{`Couldn't load ${unloaded.join(', ')}. Reload the page to try again.`}</AlertDescription>
                      </Alert>
                    ) : null}
                    <div className="grid gap-1.5">
                      <Label htmlFor="flow-name">Flow name</Label>
                      <Input
                        id="flow-name"
                        value={name}
                        onChange={(e) => {
                          checkpoint('name')
                          setName(e.target.value)
                        }}
                        placeholder="Welcome series"
                      />
                    </div>
                    <div className="grid gap-1.5">
                      <p className="text-sm font-medium">Starts when</p>
                      <div className="flex items-center justify-between gap-2 rounded-md border py-1.5 pl-3 pr-1.5">
                        <span className="flex min-w-0 items-center gap-2 text-sm">
                          <StepIcon type="trigger" className="size-4 shrink-0 text-muted-foreground" />
                          <span className="truncate">{TRIGGER_LABELS[triggerType] ?? triggerType}</span>
                        </span>
                        <Button type="button" variant="ghost" size="sm" onClick={() => selectStep(nodes.find((n) => n.type === 'trigger')?.id ?? null)}>
                          Change
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {transactional ? 'A transactional flow: it sends regardless of marketing consent.' : 'A marketing flow: its messages go only to customers who opted in.'}
                        {enrollmentFilter ? ' Only customers who match its filter are enrolled.' : ''}
                      </p>
                    </div>
                    <div className="grid gap-1.5">
                      <p className="text-sm font-medium">Status</p>
                      <div className="flex items-start gap-2">
                        <Badge variant={STATUS_VARIANT[status] ?? 'outline'} className="capitalize">
                          {status}
                        </Badge>
                        <p className="text-xs text-muted-foreground">{STATUS_HELP[status] ?? ''}</p>
                      </div>
                    </div>
                    <div className="grid gap-1.5">
                      <p className="text-sm font-medium">Before it runs</p>
                      {checks.length === 0 ? (
                        <p className="flex items-center gap-1.5 text-xs text-success">
                          <CircleCheckIcon className="size-3.5 shrink-0" />
                          Nothing to fix.
                        </p>
                      ) : (
                        <ul className="grid gap-1">
                          {checks.map((c, i) => (
                            <li key={i}>
                              <button
                                type="button"
                                disabled={!c.step}
                                onClick={() => selectStep(c.step)}
                                className="flex w-full items-start gap-2 rounded-md border px-2.5 py-2 text-left text-xs transition-colors enabled:hover:bg-muted"
                              >
                                <CheckMark level={c.level} />
                                <span className="min-w-0">
                                  <span className="font-medium text-foreground">{stepName(c.step)}.</span> <span className="text-muted-foreground">{c.text}</span>
                                </span>
                              </button>
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                  </>
                ),
              },
            ]}
          />
        </SidePanel>
      }
    >
      <div
        data-flow-canvas
        className="@container/canvas relative h-full min-h-0 w-full"
        onDragOver={(e) => {
          if (!e.dataTransfer.types.includes(STEP_DRAG)) return
          e.preventDefault()
          e.dataTransfer.dropEffect = 'copy'
        }}
        onDrop={onDropStep}
      >
        <ToolRow className="pointer-events-none absolute inset-x-0 top-3 z-10 *:pointer-events-auto">
          <ToolGroup label="Canvas">
            <ToolButton label="Zoom out" onClick={() => void zoomOut()}>
              <ZoomOutIcon className="size-3.5" />
            </ToolButton>
            <ToolButton label="Fit the flow in view" onClick={() => void fitView({ ...FIT, duration: 200 })}>
              <MaximizeIcon className="size-3.5" />
            </ToolButton>
            <ToolButton label="Zoom in" onClick={() => void zoomIn()}>
              <ZoomInIcon className="size-3.5" />
            </ToolButton>
            <ToolButton label="Tidy up: line the steps up under the trigger" onClick={tidyUp}>
              <NetworkIcon className="size-3.5" />
            </ToolButton>
          </ToolGroup>
          <ToolPill label="Steps">{nodes.filter((n) => n.type !== 'trigger').length}</ToolPill>
          <ToolPill
            label="Checks"
            onClick={() => {
              selectStep(null)
              setTab('flow')
            }}
          >
            {errors > 0 ? (
              <span className="text-destructive">{errors} to fix</span>
            ) : checks.length > 0 ? (
              <span className="text-warning">{checks.length} to look at</span>
            ) : (
              <span className="text-success">Ready</span>
            )}
          </ToolPill>
          <FlowHelp />
        </ToolRow>
        <FlowCanvasContext.Provider value={canvas}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            nodeTypes={nodeTypes}
            onNodeClick={(_, n) => setSelId(n.id)}
            onEdgeClick={() => setSelId(null)}
            onPaneClick={() => setSelId(null)}
            onNodeDragStart={() => checkpoint()}
            // The delete key goes through removeSelection, so it keeps the trigger and lands in the undo history
            deleteKeyCode={null}
            fitView
            fitViewOptions={FIT}
            proOptions={{ hideAttribution: true }}
            style={{ background: 'transparent' }}
          >
            <Background />
          </ReactFlow>
        </FlowCanvasContext.Provider>
        {recipes.length > 0 ? (
          <div className="absolute inset-x-0 bottom-4 z-10">
            <FlowStart recipes={recipes} onPick={startFrom} />
          </div>
        ) : null}
      </div>
    </DetailPageLayout>
  )
}

function FlowHelp() {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="icon" className="size-8" aria-label="How to build a flow" title="How to build a flow">
          <CircleHelpIcon className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-3">
        <p className="mb-2 text-xs font-medium">Building a flow</p>
        <ul className="mb-3 grid gap-1.5 text-xs text-muted-foreground">
          <li>Click a step on the left and it joins the flow under the selected step, or at the end.</li>
          <li>Drag one onto the canvas to place it yourself, then drag from the dot under a step to the next one.</li>
          <li>A conditional split has two dots: yes and no.</li>
        </ul>
        <ul className="grid gap-1.5">
          {FLOW_SHORTCUTS.map((s) => (
            <li key={s.what} className="flex items-center justify-between gap-3 text-xs">
              <span className="text-muted-foreground">{s.what}</span>
              <span className="flex shrink-0 gap-0.5">
                {s.keys.map((k) => (
                  <kbd key={k} className="rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                    {k}
                  </kbd>
                ))}
              </span>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
