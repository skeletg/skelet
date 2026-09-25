import { useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useUnsavedFields } from '@/lib/unsaved-changes'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
} from '@dnd-kit/core'
import { arrayMove } from '@dnd-kit/sortable'
import {
  HistoryIcon,
  MonitorIcon,
  MoonIcon,
  Redo2Icon,
  SmartphoneIcon,
  SparklesIcon,
  SunIcon,
  Undo2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { CampaignMergeTagsDocument } from '@/features/campaigns/Campaigns.gql'
import { FlowMergeTagsDocument } from '@/features/flows/Flows.gql'
import { BlockPalette } from '@/features/email/BlockPalette'
import { EmailCanvas, END_DROP, type CanvasItem } from '@/features/email/EmailCanvas'
import { BlockInspector, type AiRewriteFn } from '@/features/email/BlockInspector'
import { BlockStyleControls, GlobalStyleControls } from '@/features/email/StyleControls'
import { RowInspector } from '@/features/email/RowInspector'
import {
  addToColumn,
  duplicateBlock,
  findBlock,
  moveChild,
  removeBlock,
  replaceBlock,
} from '@/features/email/treeops'
import {
  componentMap,
  newBlock,
  newBlockRef,
  shortPreview,
  type BlockComponent,
  type EmailBlock,
  type EmailStyles,
} from '@/features/email/blocks'
import { EmailBlocksDocument, SaveEmailBlockDocument } from '@/features/email/Email.gql'
import { toComponent } from '@/features/blocks/model'
import {
  EmailGroupsDocument,
  EmailPreflightDocument,
  EmailRevisionsDocument,
  EmailsAiStatusDocument,
  LibraryEmailDocument,
  PreviewCustomersDocument,
  RenderLibraryPreviewDocument,
  RestoreEmailRevisionDocument,
  RewriteEmailTextDocument,
  SaveLibraryEmailDocument,
  SuggestEmailSubjectsDocument,
} from './Emails.gql'

let uid = 0
const nextId = () => `le_${++uid}`

// The sentinel Select value for "no group" - Radix Select forbids ""
const NO_GROUP = '__none__'

// Merge tags offered in the inspector's personalization inserter Merge tags come from the server (campaignMergeTags + flowMergeTags) - a
// library email can be sent from either a campaign or a flow, so only the union is offerable and a hardcoded list drifts
function useMergeTags(): string[] {
  const campaign = useQuery(CampaignMergeTagsDocument)
  const flow = useQuery(FlowMergeTagsDocument)
  return useMemo(() => {
    const all = [
      ...(campaign.data?.campaignMergeTags ?? []),
      ...(flow.data?.flowMergeTags ?? []),
    ]
    return [...new Set(all)]
  }, [campaign.data, flow.data])
}

type GroupOption = { id: string; slug: string; name: string }

type Loaded = {
  id: string | null
  channelId: string
  name: string
  subject: string
  preheader: string
  blocks: EmailBlock[]
  styles: EmailStyles
  groupId: string | null
}

// The editable document - the unit undo/redo snapshots
type Doc = {
  name: string
  subject: string
  preheader: string
  groupId: string | null
  items: CanvasItem[]
  styles: EmailStyles
}

function itemsFromBlocks(blocks: EmailBlock[]): CanvasItem[] {
  return blocks.map((b) => ({ id: nextId(), block: b }))
}

export function EmailLibraryDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const isNew = !id || id === 'new'

  const { data, loading } = useQuery(LibraryEmailDocument, { variables: { id: id ?? '' }, skip: isNew, fetchPolicy: 'cache-and-network' })
  const { data: groupsData } = useQuery(EmailGroupsDocument, { fetchPolicy: 'cache-and-network' })
  const groups: GroupOption[] = groupsData?.emailGroups ?? []
  // New emails default to the Marketing group; the operator can retag as needed
  const defaultGroupId = groups.find((g) => g.slug === 'marketing')?.id ?? null

  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="New email" backTo="/emails" />
        <div className="p-6">
          <Alert>
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>Select a channel in the header before building an email.</AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }
  if (!isNew && !data?.libraryEmail) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Email" backTo="/emails" />
        <div className="p-6">{loading ? <LoadingState /> : null}</div>
      </DetailPageLayout>
    )
  }

  const initial: Loaded = isNew
    ? { id: null, channelId: effectiveChannelId ?? '', name: '', subject: '', preheader: '', blocks: [], styles: {}, groupId: defaultGroupId }
    : {
        id: data!.libraryEmail!.id,
        channelId: data!.libraryEmail!.channelId,
        name: data!.libraryEmail!.name,
        subject: data!.libraryEmail!.subject,
        preheader: data!.libraryEmail!.preheader ?? '',
        blocks: (data!.libraryEmail!.blocks as EmailBlock[] | null) ?? [],
        styles: (data!.libraryEmail!.styles as EmailStyles | null) ?? {},
        groupId: data!.libraryEmail!.groupId ?? null,
      }

  // Remount the editor once the default group resolves (new) or per email (edit)
  // so the initial group is applied without setState-in-effect
  const editorKey = isNew ? `new-${defaultGroupId ?? 'none'}` : (initial.id ?? 'new')

  return <Editor key={editorKey} initial={initial} groups={groups} onCreated={(nid) => void duringCommit(() => navigate(`/emails/${nid}`))} />
}

// Undo/redo state - present doc plus bounded past/future stacks
type Hist = { past: Doc[]; present: Doc; future: Doc[] }
const HISTORY_LIMIT = 100

function Editor({ initial, groups, onCreated }: { initial: Loaded; groups: GroupOption[]; onCreated: (id: string) => void }) {
  const mergeTags = useMergeTags()
  const [hist, setHist] = useState<Hist>(() => ({
    past: [],
    present: {
      name: initial.name,
      subject: initial.subject,
      preheader: initial.preheader,
      groupId: initial.groupId,
      items: itemsFromBlocks(initial.blocks),
      styles: initial.styles,
    },
    future: [],
  }))
  const doc = hist.present
  // Coalesce consecutive edits of the same field (e.g. typing) into one undo step
  const coalesceKey = useRef<string | null>(null)

  function commit(update: (d: Doc) => Doc, coalesce?: string) {
    setHist((h) => {
      const next = update(h.present)
      if (next === h.present) return h
      if (coalesce && coalesce === coalesceKey.current) {
        return { ...h, present: next, future: [] }
      }
      coalesceKey.current = coalesce ?? null
      return { past: [...h.past, h.present].slice(-HISTORY_LIMIT), present: next, future: [] }
    })
  }
  function undo() {
    coalesceKey.current = null
    setHist((h) => (h.past.length === 0 ? h : { past: h.past.slice(0, -1), present: h.past[h.past.length - 1], future: [h.present, ...h.future] }))
  }
  function redo() {
    coalesceKey.current = null
    setHist((h) => (h.future.length === 0 ? h : { past: [...h.past, h.present], present: h.future[0], future: h.future.slice(1) }))
  }
  // Replace the whole document in one history step (restore from a revision)
  function replaceDoc(next: Doc) {
    coalesceKey.current = null
    setHist((h) => ({ past: [...h.past, h.present].slice(-HISTORY_LIMIT), present: next, future: [] }))
  }

  const [sel, setSel] = useState<string | null>(null)
  const [dropBefore, setDropBefore] = useState<string | null>(null)
  const [emailId, setEmailId] = useState<string | null>(initial.id)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [saveBlockFor, setSaveBlockFor] = useState<EmailBlock | null>(null)

  const blocks = useMemo(() => doc.items.map((i) => i.block), [doc.items])
  const { data: blocksData } = useQuery(EmailBlocksDocument, { variables: { channelId: initial.channelId } })
  const components: BlockComponent[] = useMemo(() => (blocksData?.emailBlocks ?? []).map(toComponent), [blocksData])
  const compMap = useMemo(() => componentMap(components), [components])
  const snippetList = components.map((c) => ({ slug: c.slug, name: c.name }))

  const { data: aiStatusData } = useQuery(EmailsAiStatusDocument, { fetchPolicy: 'cache-first' })
  const aiEnabled = aiStatusData?.aiStatus.enabled ?? false

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const selectedBlock = findBlock(doc.items, sel)
  const selectedIsRow = !!selectedBlock && selectedBlock.type === 'row' && !!sel && !sel.includes('::')

  function makeBlock(type: string): EmailBlock {
    const c = compMap[type]
    return c ? newBlockRef(c) : newBlock(type)
  }
  function setItems(updater: (items: CanvasItem[]) => CanvasItem[], coalesce?: string) {
    commit((d) => ({ ...d, items: updater(d.items) }), coalesce)
  }
  function deleteSel(target: string) {
    setItems((prev) => removeBlock(prev, target))
    setSel((cur) => (cur === target || (cur && cur.startsWith(`${target}::`)) ? null : cur))
  }
  function duplicateSel(target: string) {
    setItems((prev) => duplicateBlock(prev, target, nextId))
  }
  function updateSelected(next: EmailBlock) {
    if (sel) setItems((prev) => replaceBlock(prev, sel, next), `block:${sel}`)
  }
  function onMoveChild(rowId: string, col: number, idx: number, dir: -1 | 1) {
    setItems((prev) => moveChild(prev, rowId, col, idx, dir))
  }
  // Track where a dragged palette block will land, to draw the insertion line
  function onDragOver(e: DragOverEvent) {
    const ad = e.active.data.current
    if (!ad?.fromPalette) {
      if (dropBefore !== null) setDropBefore(null)
      return
    }
    const overId = e.over ? String(e.over.id) : null
    if (!overId || overId.startsWith('col:')) {
      setDropBefore(null) // over a column drop-zone (it highlights itself) or nothing
      return
    }
    setDropBefore(overId === 'canvas' ? END_DROP : overId)
  }
  function onDragEnd(e: DragEndEvent) {
    setDropBefore(null)
    const { active, over } = e
    if (!over) return
    const overId = String(over.id)
    const ad = active.data.current
    if (ad?.fromPalette) {
      const type = String(ad.blockType)
      // Components (dropped as refs) + rows can't nest inside a column
      if (overId.startsWith('col:') && type !== 'row' && type !== 'ref' && !compMap[type]) {
        const [, rowId, colStr] = overId.split(':')
        setItems((prev) => addToColumn(prev, rowId, Number(colStr), makeBlock(type)))
        return
      }
      const item = { id: nextId(), block: makeBlock(type) }
      setItems((prev) => {
        const overIdx = prev.findIndex((i) => i.id === overId)
        if (overIdx === -1) return [...prev, item]
        const next = [...prev]
        next.splice(overIdx, 0, item)
        return next
      })
      setSel(item.id)
      return
    }
    if (active.id !== over.id) {
      setItems((prev) => {
        const from = prev.findIndex((i) => i.id === active.id)
        const to = prev.findIndex((i) => i.id === overId)
        if (from === -1 || to === -1) return prev
        return arrayMove(prev, from, to)
      })
    }
  }

  // Pre-send checklist (deliverability + quality warnings) - only for a saved
  // email - Refetched after each successful save so the list reflects edits
  const { data: preflightData, refetch: refetchPreflight } = useQuery(EmailPreflightDocument, {
    variables: { id: emailId ?? '' },
    skip: !emailId,
    fetchPolicy: 'cache-and-network',
  })
  const warnings = preflightData?.emailPreflight ?? []

  const [saveEmail] = useMutation(SaveLibraryEmailDocument)
  // hist.present is the live design - the undo stacks around it are not unsaved work
  useUnsavedFields([hist.present])

  async function onSave() {
    if (!doc.name.trim()) {
      setError('Give the email a name.')
      return
    }
    setError(null)
    setSaving(true)
    try {
      const res = await saveEmail({
        variables: { input: { id: emailId, channelId: initial.channelId, name: doc.name.trim(), subject: doc.subject, preheader: doc.preheader, blocks, styles: doc.styles, groupId: doc.groupId } },
      })
      const nid = res.data?.saveLibraryEmail?.id ?? null
      if (nid && !emailId) {
        setEmailId(nid)
        onCreated(nid)
      } else if (emailId) {
        // Re-run the checklist against the just-saved content
        void refetchPreflight()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  // AI copy assist (only wired when AI is configured)
  const [rewriteText] = useMutation(RewriteEmailTextDocument)
  const aiRewrite: AiRewriteFn | undefined = aiEnabled
    ? async (text, instruction) => {
        const res = await rewriteText({ variables: { text, instruction } })
        return res.data?.rewriteEmailText ?? text
      }
    : undefined

  // Restore a prior version → load its content into the editor as one undo step
  const [restoreRevision] = useMutation(RestoreEmailRevisionDocument)
  async function onRestore(revisionId: string) {
    const res = await restoreRevision({ variables: { revisionId } })
    const e = res.data?.restoreEmailRevision
    if (!e) return
    replaceDoc({
      name: e.name,
      subject: e.subject,
      preheader: e.preheader ?? '',
      groupId: e.groupId ?? null,
      items: itemsFromBlocks((e.blocks as EmailBlock[] | null) ?? []),
      styles: (e.styles as EmailStyles | null) ?? {},
    })
    setSel(null)
    setHistoryOpen(false)
    void refetchPreflight()
  }

  // Pinned action bar for the right sidebar
  const actionsFooter = (
    <div className="space-y-2">
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex items-center gap-1">
        <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Undo" disabled={hist.past.length === 0} onClick={undo}>
          <Undo2Icon className="size-4" />
        </Button>
        <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Redo" disabled={hist.future.length === 0} onClick={redo}>
          <Redo2Icon className="size-4" />
        </Button>
        {emailId && (
          <Button type="button" variant="outline" size="icon" className="size-8 shrink-0" aria-label="Version history" onClick={() => setHistoryOpen(true)}>
            <HistoryIcon className="size-4" />
          </Button>
        )}
        <Button type="button" variant="outline" size="sm" className="flex-1" onClick={() => setPreviewOpen(true)}>
          Preview
        </Button>
        <Button type="button" size="sm" className="flex-1" disabled={saving} onClick={onSave}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
      onDragCancel={() => setDropBefore(null)}
    >
      <DetailPageLayout
        leftSidebar={<BlockPalette allowData={false} components={components} />}
        rightSidebar={
          selectedIsRow && selectedBlock ? (
            <aside className="flex w-80 shrink-0 flex-col border-l bg-card">
              <div className="border-b px-4 py-3">
                <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Edit Columns</h2>
              </div>
              <div className="flex-1 overflow-y-auto p-4">
                <RowInspector block={selectedBlock} onChange={updateSelected} />
              </div>
              <div className="shrink-0 border-t p-3">{actionsFooter}</div>
            </aside>
          ) : (
            <BlockInspector
              block={selectedBlock}
              onChangeBlock={updateSelected}
              subject={doc.subject}
              onSubjectChange={(s) => commit((d) => ({ ...d, subject: s }), 'subject')}
              mergeTags={mergeTags}
              snippets={snippetList}
              components={compMap}
              showSubject
              aiRewrite={aiRewrite}
              styleControls={
                selectedBlock ? (
                  <>
                    <BlockStyleControls block={selectedBlock} onChange={updateSelected} />
                    {sel && !sel.includes('::') && selectedBlock.type !== 'ref' && (
                      <Button type="button" variant="outline" size="sm" className="w-full" onClick={() => setSaveBlockFor(selectedBlock)}>
                        Save as reusable block
                      </Button>
                    )}
                  </>
                ) : null
              }
              emailSettings={
                <div className="grid gap-4">
                  {aiEnabled && (
                    <SubjectAssist
                      subject={doc.subject}
                      name={doc.name}
                      preheader={doc.preheader}
                      blocks={blocks}
                      onPick={(s) => commit((d) => ({ ...d, subject: s }))}
                    />
                  )}
                  <div className="grid gap-1.5">
                    <Label htmlFor="email-preheader">Preheader</Label>
                    <Input
                      id="email-preheader"
                      value={doc.preheader}
                      onChange={(e) => commit((d) => ({ ...d, preheader: e.target.value }), 'preheader')}
                      placeholder="Inbox-preview snippet"
                    />
                    <p className="text-xs text-muted-foreground">
                      Shown after the subject in the inbox preview. Supports merge tags.
                    </p>
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="email-name">Email name</Label>
                    <Input id="email-name" value={doc.name} onChange={(e) => commit((d) => ({ ...d, name: e.target.value }), 'name')} placeholder="Welcome hero" />
                  </div>
                  <div className="grid gap-1.5">
                    <Label htmlFor="email-group">Group</Label>
                    <Select value={doc.groupId ?? NO_GROUP} onValueChange={(v) => commit((d) => ({ ...d, groupId: v === NO_GROUP ? null : v }))}>
                      <SelectTrigger id="email-group">
                        <SelectValue placeholder="No group" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_GROUP}>No group</SelectItem>
                        {groups.map((g) => (
                          <SelectItem key={g.id} value={g.id}>
                            {g.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  {emailId && <PreflightPanel warnings={warnings} />}
                  <GlobalStyleControls value={doc.styles} onChange={(s) => commit((d) => ({ ...d, styles: s }))} />
                </div>
              }
              footer={actionsFooter}
            />
          )
        }
      >
        <EmailCanvas
          items={doc.items}
          sel={sel}
          onSelect={setSel}
          onDelete={deleteSel}
          onDuplicate={duplicateSel}
          onMoveChild={onMoveChild}
          theme={doc.styles}
          components={compMap}
          subject={doc.subject}
          emailFrame
          dropBefore={dropBefore}
        />
      </DetailPageLayout>

      <PreviewDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        channelId={initial.channelId}
        subject={doc.subject}
        preheader={doc.preheader}
        blocks={blocks}
        styles={doc.styles}
      />

      <HistoryDialog open={historyOpen} onOpenChange={setHistoryOpen} emailId={emailId} onRestore={onRestore} />

      <SaveAsBlockDialog block={saveBlockFor} channelId={initial.channelId} onClose={() => setSaveBlockFor(null)} />
    </DndContext>
  )
}

// PreflightPanel renders the pre-send checklist: warn findings in a destructive
// tone, info findings muted - Empty = all checks passed
function PreflightPanel({ warnings }: { warnings: Array<{ severity: string; message: string }> }) {
  return (
    <div className="grid gap-1.5">
      <Label>Pre-send checklist</Label>
      {warnings.length === 0 ? (
        <p className="rounded-md border bg-muted/30 px-2.5 py-2 text-xs text-muted-foreground">All checks passed ✓</p>
      ) : (
        <ul className="grid gap-1.5">
          {warnings.map((w, i) => (
            <li
              key={i}
              className={
                w.severity === 'warn'
                  ? 'rounded-md border border-amber-300 bg-amber-50 px-2.5 py-1.5 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300'
                  : 'rounded-md border bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground'
              }
            >
              {w.message}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// SubjectAssist offers AI subject-line suggestions built from the email's name,
// preheader and copy - Clicking a suggestion sets the subject
function SubjectAssist({
  subject,
  name,
  preheader,
  blocks,
  onPick,
}: {
  subject: string
  name: string
  preheader: string
  blocks: EmailBlock[]
  onPick: (s: string) => void
}) {
  const [suggest] = useMutation(SuggestEmailSubjectsDocument)
  const [busy, setBusy] = useState(false)
  const [ideas, setIdeas] = useState<string[]>([])
  const [err, setErr] = useState<string | null>(null)

  function buildContext(): string {
    const copy = blocks
      .flatMap((b) => [b.text, b.content, b.label])
      .filter((s): s is string => !!s && s.trim() !== '')
      .join(' ')
    return [name && `Email: ${name}`, subject && `Current subject: ${subject}`, preheader && `Preheader: ${preheader}`, copy && `Body: ${copy}`]
      .filter(Boolean)
      .join('\n')
      .slice(0, 2000)
  }

  async function run() {
    setBusy(true)
    setErr(null)
    try {
      const res = await suggest({ variables: { context: buildContext(), count: 5 } })
      setIdeas(res.data?.suggestEmailSubjects ?? [])
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not generate suggestions.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-2 rounded-md border bg-muted/20 p-2.5">
      <div className="flex items-center justify-between">
        <Label className="text-xs">Subject ideas</Label>
        <Button type="button" variant="outline" size="sm" className="h-7" disabled={busy} onClick={run}>
          <SparklesIcon className="mr-1 size-3.5" />
          {busy ? 'Thinking…' : 'Suggest'}
        </Button>
      </div>
      {err && <p className="text-xs text-destructive">{err}</p>}
      {ideas.length > 0 && (
        <ul className="grid gap-1">
          {ideas.map((idea, i) => (
            <li key={i}>
              <button
                type="button"
                onClick={() => onPick(idea)}
                className="w-full rounded border bg-background px-2 py-1 text-left text-xs hover:bg-accent hover:text-accent-foreground"
              >
                {idea}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// PreviewDialog renders the email server-side, with device (desktop/mobile),
// dark-mode approximation, and optional real-customer merge-tag data
function PreviewDialog({
  open,
  onOpenChange,
  channelId,
  subject,
  preheader,
  blocks,
  styles,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  channelId: string
  subject: string
  preheader: string
  blocks: EmailBlock[]
  styles: EmailStyles
}) {
  const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop')
  const [dark, setDark] = useState(false)
  const [customerId, setCustomerId] = useState<string | null>(null)

  const { data: custData } = useQuery(PreviewCustomersDocument, { variables: { channelId }, skip: !open, fetchPolicy: 'cache-first' })
  const customers = custData?.customersPaged.items ?? []

  const { data, loading } = useQuery(RenderLibraryPreviewDocument, {
    variables: { channelId, customerId, subject, preheader, blocks, styles },
    skip: !open,
    fetchPolicy: 'no-cache',
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Preview</DialogTitle>
          <DialogDescription>Rendered server-side. Choose a device, dark mode, or a real customer’s data.</DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2">
          <div className="flex rounded-md border p-0.5">
            <button
              type="button"
              onClick={() => setDevice('desktop')}
              className={cn('flex items-center gap-1 rounded px-2 py-1 text-xs', device === 'desktop' ? 'bg-accent text-accent-foreground' : 'text-muted-foreground')}
            >
              <MonitorIcon className="size-3.5" /> Desktop
            </button>
            <button
              type="button"
              onClick={() => setDevice('mobile')}
              className={cn('flex items-center gap-1 rounded px-2 py-1 text-xs', device === 'mobile' ? 'bg-accent text-accent-foreground' : 'text-muted-foreground')}
            >
              <SmartphoneIcon className="size-3.5" /> Mobile
            </button>
          </div>
          <Button type="button" variant="outline" size="sm" className="h-8" onClick={() => setDark((d) => !d)}>
            {dark ? <SunIcon className="mr-1 size-3.5" /> : <MoonIcon className="mr-1 size-3.5" />}
            {dark ? 'Light' : 'Dark'}
          </Button>
          <Select value={customerId ?? NO_GROUP} onValueChange={(v) => setCustomerId(v === NO_GROUP ? null : v)}>
            <SelectTrigger className="h-8 w-56 text-xs">
              <SelectValue placeholder="Sample data" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_GROUP}>Sample data</SelectItem>
              {customers.map((c) => (
                <SelectItem key={c.id} value={c.id}>
                  {[c.firstName, c.lastName].filter(Boolean).join(' ') || c.email}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className={cn('rounded-md p-3', dark ? 'bg-neutral-900' : 'bg-muted/40')}>
          <div className="mb-2 text-xs" style={{ color: dark ? '#ddd' : undefined }}>
            <span className="font-medium">Subject:</span> {loading ? '…' : data?.renderLibraryPreview.subject || '-'}
          </div>
          <div className="mx-auto overflow-hidden rounded border bg-white" style={{ maxWidth: device === 'mobile' ? 390 : '100%' }}>
            <iframe
              title="Email preview"
              sandbox=""
              className="h-[440px] w-full bg-white"
              style={dark ? { filter: 'invert(1) hue-rotate(180deg)' } : undefined}
              srcDoc={data?.renderLibraryPreview.html ?? ''}
            />
          </div>
          {dark && <p className="mt-2 text-center text-[11px]" style={{ color: '#999' }}>Dark mode is an approximation (color inversion).</p>}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// HistoryDialog lists an email's saved versions and restores one
function HistoryDialog({
  open,
  onOpenChange,
  emailId,
  onRestore,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  emailId: string | null
  onRestore: (revisionId: string) => Promise<void>
}) {
  const { data, loading } = useQuery(EmailRevisionsDocument, {
    variables: { id: emailId ?? '' },
    skip: !open || !emailId,
    fetchPolicy: 'cache-and-network',
  })
  const [busyId, setBusyId] = useState<string | null>(null)
  const revisions = data?.emailRevisions ?? []

  async function restore(id: string) {
    setBusyId(id)
    try {
      await onRestore(id)
    } finally {
      setBusyId(null)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Version history</DialogTitle>
          <DialogDescription>Each save records a version. Restore one to bring its content back (recorded as a new version).</DialogDescription>
        </DialogHeader>
        {loading && revisions.length === 0 ? (
          <LoadingState />
        ) : revisions.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">No versions yet.</p>
        ) : (
          <ul className="max-h-96 divide-y overflow-y-auto">
            {revisions.map((r, i) => (
              <li key={r.id} className="flex items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {r.subject || <span className="text-muted-foreground">(no subject)</span>}
                    {i === 0 && <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">latest</span>}
                  </p>
                  <p className="text-xs text-muted-foreground">{formatWhen(r.createdAt)}</p>
                </div>
                <Button type="button" variant="outline" size="sm" disabled={busyId !== null} onClick={() => void restore(r.id)}>
                  {busyId === r.id ? 'Restoring…' : 'Restore'}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  )
}

function formatWhen(v: unknown): string {
  const d = new Date(String(v))
  return Number.isNaN(d.getTime()) ? String(v) : d.toLocaleString()
}

// SaveAsBlockDialog saves the selected block as a reusable library block
function SaveAsBlockDialog({
  block,
  channelId,
  onClose,
}: {
  block: EmailBlock | null
  channelId: string
  onClose: () => void
}) {
  const [name, setName] = useState('')
  const [saveBlock] = useMutation(SaveEmailBlockDocument)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

  async function save() {
    if (!slug) {
      setErr('Give the block a name.')
      return
    }
    setErr(null)
    setSaving(true)
    try {
      await saveBlock({ variables: { input: { channelId, slug, name: name.trim(), blocks: block ? [block] : [] } } })
      onClose()
      setName('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={!!block} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Save as reusable block</DialogTitle>
          <DialogDescription>Adds this block to the channel’s reusable library so you can drop it into other emails.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          {block && <p className="rounded-md border bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground">{shortPreview(block) || block.type}</p>}
          <div className="grid gap-1.5">
            <Label htmlFor="block-name">Name</Label>
            <Input id="block-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Promo hero" autoFocus />
            {slug && <p className="font-mono text-xs text-muted-foreground">{slug}</p>}
          </div>
          {err && <p className="text-xs text-destructive">{err}</p>}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="button" disabled={saving} onClick={save}>
            {saving ? 'Saving…' : 'Save block'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
