import { useEffect, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, rectSortingStrategy, sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { CopyIcon, GripVerticalIcon, Maximize2Icon, Minimize2Icon, PencilIcon, PlusIcon, Share2Icon, StarIcon, Trash2Icon, XIcon } from 'lucide-react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { ChannelRequired } from '@/shared/analytics/ChannelRequired'
import { PeriodPicker } from '@/shared/analytics/PeriodPicker'
import { BUILDERS, ReportControls, ReportResult, suggestTitle } from '@/shared/analytics/kinds'
import { DEFAULT_DEFINITIONS, REPORT_KIND_LABEL, REPORT_SIZE_LABEL, describeReport, type BoardReport, type ReportDefinition, type ReportKind, type ReportSize } from '@/shared/analytics/report'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { added } from '@/shared/saving/diff'
import { useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import {
  BoardDocument,
  BoardsDocument,
  ClaimBoardDocument,
  DeleteBoardDocument,
  DiscardBoardDraftDocument,
  MakeShopBoardDocument,
  SaveBoardDocument,
  SaveBoardDraftDocument,
  SetBoardFavouriteDocument,
  type BoardFullFragment,
} from './Boards.gql'
import { ShareDialog } from './ShareDialog'
import { boardDiff, keyed, unkeyed, type AddToBoardState, type DraftReport } from './boardDiff'

const newKey = () => `n${crypto.randomUUID()}` // a report added since the board was loaded - boardDiff reads it as new

// A tile's width in the board's grid, which is 1 column narrow, 2 from a medium width, 4 wide and 5 very wide - measured
// on the grid itself, so opening the edit panel narrows it rather than squeezing four tiles into the space left
const SPAN: Record<ReportSize, string> = { small: 'col-span-1', medium: 'col-span-1 @2xl:col-span-2', full: 'col-span-full' }

// BoardPage is the board as a canvas: tiles are added, built, resized and reordered in place, the edit panel sits beside
// the grid while the tile previews each change, and the header Save applies the lot
export function BoardPage() {
  const { id = '' } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { canAccessChannelScope } = usePermissions()
  const canManage = isChannel && canAccessChannelScope('boards.manage', channelId)
  // Tiles run as the viewer: without customer analytics here they'd only error, so they say what's missing instead
  const seesData = isChannel && canAccessChannelScope('customers.view', channelId)
  const { data, loading, error } = useQuery(BoardDocument, { variables: { channelId, id }, skip: !isChannel || !id })
  const board = data?.board
  const saved: BoardReport[] = (board?.reports ?? []) as BoardReport[]
  const canEdit = board?.access === 'edit' || board?.access === 'own'
  const canOwn = board?.access === 'own'

  const [draftName, setDraftName] = useState('')
  const [draft, setDraft] = useState<DraftReport[]>([])
  const [sinceDays, setSinceDays] = useState(30)
  const [loadedId, setLoadedId] = useState('') // the board the draft holds - until it's this one there's nothing to diff
  const [editing, setEditing] = useState<string | null>(null) // the key of the tile the panel edits
  const [focused, setFocused] = useState(false) // the edited tile on its own, full width
  const [arriving] = useState(() => (location.state as AddToBoardState | null)?.add) // what Add to board sent, read once
  const [base, setBase] = useState('') // the board's updatedAt the staged edits started from - Save sends it as readAt
  const [restoredAt, setRestoredAt] = useState<string | null>(null) // when the kept draft this page opened with was written
  // Adopt the board on its identity only, so a refetch never wipes edits mid-way. Unsaved edits kept from an earlier
  // visit come back as they were staged, against the board they started from
  if (board && loadedId !== board.id) {
    setLoadedId(board.id)
    const kept = canEdit ? readDraft(board.draft?.payload) : null
    setDraftName(kept?.name ?? board.name)
    setBase(kept ? board.draft!.baseUpdatedAt : board.updatedAt)
    setRestoredAt(kept ? board.draft!.updatedAt : null)
    const reports = kept?.reports ?? keyed(board.reports as BoardReport[])
    if (arriving) {
      const key = newKey()
      setDraft([...reports, { ...arriving, key }])
      setEditing(key)
    } else setDraft(reports)
  }
  useEffect(() => {
    if (arriving) navigate(location.pathname, { replace: true, state: null }) // a refresh mustn't add it a second time
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const registry = useSaveRegistry(board ? `Board · ${board.name}` : 'Board')
  const [saveBoard] = useMutation(SaveBoardDocument, { refetchQueries: [BoardsDocument] })
  const [keepDraft] = useMutation(SaveBoardDraftDocument)
  const [dropDraft] = useMutation(DiscardBoardDraftDocument, { refetchQueries: [BoardsDocument] })
  const [star] = useMutation(SetBoardFavouriteDocument, { refetchQueries: [BoardsDocument, BoardDocument] })
  const [makeShop] = useMutation(MakeShopBoardDocument, { refetchQueries: [BoardsDocument] })
  const [claim, claiming] = useMutation(ClaimBoardDocument, { refetchQueries: [BoardsDocument] })
  const [sharing, setSharing] = useState(false)
  const [confirmingShop, setConfirmingShop] = useState(false)
  const [deleteBoard] = useMutation(DeleteBoardDocument, { refetchQueries: [BoardsDocument] })
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const changes = board && loadedId === board.id ? boardDiff(board.name, saved, draftName, draft) : []
  // Unsaved edits are kept on the server about a second after the last one, so a reload or another device picks them
  // up - they change nothing anyone else sees, which is why this is the one write that doesn't wait for Save
  const changesKey = JSON.stringify(changes)
  useEffect(() => {
    if (!board || loadedId !== board.id || !canEdit || !base) return
    const t = setTimeout(() => {
      if (changes.length > 0) void keepDraft({ variables: { channelId, id: board.id, payload: { name: draftName, reports: draft }, baseUpdatedAt: base } }).catch(() => {})
      else if (restoredAt || board.draft) void dropDraft({ variables: { channelId, id: board.id } }).catch(() => {})
    }, DRAFT_DELAY_MS)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changesKey, loadedId, canEdit])
  const publish = board?.status === 'draft' ? [added('Publish', 'Save it to keep it - it stays private to you until you share it')] : []

  useSaveSection(
    {
      id: 'board',
      title: 'Board',
      diff: board && loadedId === board.id ? [...publish, ...changes] : [],
      save: async () => {
        if (!board) return
        const res = await saveBoard({
          variables: { channelId, id: board.id, name: draftName.trim(), reports: unkeyed(draft).map((r) => ({ title: r.title.trim(), definition: r.definition })), readAt: base },
        })
        const next = res.data?.saveBoard
        if (next) {
          setDraft(keyed(next.reports as BoardReport[]))
          setDraftName(next.name)
          setBase(next.updatedAt)
          setRestoredAt(null)
          setEditing(null)
        }
      },
    },
    registry.contextValue,
  )

  async function onConfirmSave() {
    setSaving(true)
    setSaveError(null)
    try {
      await registry.saveAll()
      setConfirmingSave(false)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Could not save the board')
    } finally {
      setSaving(false)
    }
  }

  const update = (key: string, patch: Partial<BoardReport>) => setDraft((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))
  const add = (kind: ReportKind) => {
    const definition = DEFAULT_DEFINITIONS[kind]
    const key = newKey()
    setDraft((rs) => [...rs, { key, title: suggestTitle(definition), definition }])
    setEditing(key)
    setFocused(false)
  }
  const remove = (key: string) => {
    setDraft((rs) => rs.filter((r) => r.key !== key))
    if (editing === key) setEditing(null)
  }
  const duplicate = (key: string) =>
    setDraft((rs) => {
      const i = rs.findIndex((r) => r.key === key)
      return [...rs.slice(0, i + 1), { ...rs[i], key: newKey(), title: `${rs[i].title} (copy)` }, ...rs.slice(i + 1)]
    })
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }))
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    setDraft((rs) => arrayMove(rs, rs.findIndex((r) => r.key === active.id), rs.findIndex((r) => r.key === over.id)))
  }
  const current = draft.find((r) => r.key === editing)
  useEffect(() => {
    if (!current) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (focused) setFocused(false)
      else setEditing(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [current, focused])

  if (!isChannel) return <ChannelRequired what="Boards" />

  async function discardKept() {
    if (!board) return
    await dropDraft({ variables: { channelId, id: board.id } })
    setDraftName(board.name)
    setDraft(keyed(board.reports as BoardReport[]))
    setBase(board.updatedAt)
    setRestoredAt(null)
    setEditing(null)
  }

  const addMenu = (trigger: React.ReactNode) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {BUILDERS.map((b) => (
          <DropdownMenuItem key={b.kind} onSelect={() => add(b.kind)}>
            <span className="flex flex-col">
              <span>{b.label}</span>
              <span className="text-xs text-muted-foreground">{b.hint}</span>
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
  const shown = focused && current ? [current] : draft

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/boards"
        backLabel="Back to boards"
        title={
          canEdit && board ? (
            <>
              <Label htmlFor="board-name" className="sr-only">
                Board name
              </Label>
              <Input id="board-name" value={draftName} onChange={(e) => setDraftName(e.target.value)} className="h-8 w-72 text-lg font-semibold" />
            </>
          ) : (
            (board?.name ?? 'Board')
          )
        }
        badges={<Badge variant="secondary">{selectedChannel?.name}</Badge>}
        meta={board && <span className="text-xs text-muted-foreground">{sharingSummary(board, selectedChannel?.name ?? 'this shop')}</span>}
        actions={
          <div className="flex items-center gap-3">
            <PeriodPicker value={sinceDays} onChange={setSinceDays} />
            {board && (
              <Button
                size="icon"
                variant="ghost"
                aria-pressed={board.favourite}
                aria-label={board.favourite ? 'Unstar this board' : 'Star this board'}
                onClick={() => void star({ variables: { channelId, id: board.id, favourite: !board.favourite } })}
              >
                <StarIcon className={board.favourite ? 'size-4 fill-current' : 'size-4'} />
              </Button>
            )}
            {board?.owner?.removed && canManage && (
              <Button size="sm" variant="outline" disabled={claiming.loading} onClick={() => void claim({ variables: { channelId, id: board.id } })}>
                Claim it
              </Button>
            )}
            {canOwn && board?.owner && board.status === 'published' && (
              <>
                <Button size="sm" variant="outline" onClick={() => setSharing(true)}>
                  <Share2Icon className="size-4" />
                  Share
                </Button>
                {canManage && (
                  <Button size="sm" variant="ghost" onClick={() => setConfirmingShop(true)}>
                    Make it a shop board
                  </Button>
                )}
              </>
            )}
            {canEdit && board && (
              <>
                {addMenu(
                  <Button size="sm" variant="outline">
                    <PlusIcon className="size-4" />
                    Add report
                  </Button>,
                )}
                {canOwn && (
                  <Button size="sm" variant="outline" onClick={() => setConfirmingDelete(true)}>
                    <Trash2Icon className="size-4" />
                    Delete
                  </Button>
                )}
                <Button size="sm" disabled={!registry.isDirty || saving} onClick={() => setConfirmingSave(true)}>
                  Save
                </Button>
              </>
            )}
          </div>
        }
      />
      <DetailPageBody>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load the board</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {loading && !board && <p className="py-6 text-sm text-muted-foreground">Loading…</p>}
        {claiming.error && <p className="px-6 pt-4 text-sm text-destructive">{claiming.error.message}</p>}
        {restoredAt && (
          <div className="mx-6 mt-4 flex flex-wrap items-center gap-3 rounded-md border border-info/40 bg-info-soft px-3 py-2 text-sm">
            <span className="flex-1">
              These are your unsaved changes from {new Date(restoredAt).toLocaleString()} - Save to keep them. Unsaved changes are kept for 30 days.
            </span>
            <Button size="sm" variant="outline" onClick={() => void discardKept()}>
              Discard them
            </Button>
          </div>
        )}
        {board && (
          <div className="flex items-start gap-4 px-6 py-6">
            <div className="@container min-w-0 flex-1">
              <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
                <SortableContext items={shown.map((r) => r.key)} strategy={rectSortingStrategy}>
                  <div className="grid grid-cols-1 gap-4 @2xl:grid-cols-2 @5xl:grid-cols-4 @[88rem]:grid-cols-5">
                    {shown.map((r) => (
                      <Tile
                        key={r.key}
                        r={r}
                        channelId={channelId}
                        sinceDays={sinceDays}
                        canManage={canEdit}
                        seesData={seesData}
                        shop={selectedChannel?.name ?? 'this shop'}
                        editing={r.key === editing}
                        focused={focused}
                        onEdit={() => setEditing(r.key)}
                        onRemove={() => remove(r.key)}
                        onDuplicate={() => duplicate(r.key)}
                      />
                    ))}
                    {!focused &&
                      (canEdit
                        ? addMenu(
                            <button type="button" className="col-span-1 flex min-h-40 flex-col items-center justify-center gap-1 rounded-md border border-dashed text-sm text-muted-foreground hover:border-foreground/30 hover:text-foreground">
                              <PlusIcon className="size-5" />
                              Add report
                            </button>,
                          )
                        : draft.length === 0 && <p className="col-span-full py-6 text-center text-sm text-muted-foreground">No reports on this board yet.</p>)}
                  </div>
                </SortableContext>
              </DndContext>
            </div>
            {current && canEdit && (
              <EditPanel
                r={current}
                channelId={channelId}
                focused={focused}
                onChange={(patch) => update(current.key, patch)}
                onFocus={() => setFocused((f) => !f)}
                onClose={() => {
                  setEditing(null)
                  setFocused(false)
                }}
              />
            )}
          </div>
        )}
      </DetailPageBody>
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={board?.name ?? 'Board'}
        sections={registry.dirtySections}
        saving={saving}
        error={saveError}
        onCancel={() => setConfirmingSave(false)}
        onConfirm={() => void onConfirmSave()}
      />
      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={`Delete ${board?.name ?? 'this board'}?`}
        description="Its reports go with it, for everyone it's shared with. The data they show stays."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!board) return
          await deleteBoard({ variables: { channelId, id: board.id } })
          navigate('/boards')
        }}
      />
      {board && (
        <>
          <ShareDialog channelId={channelId} shop={selectedChannel?.name ?? 'this shop'} board={board} open={sharing} onOpenChange={setSharing} />
          <ConfirmDialog
            open={confirmingShop}
            onOpenChange={setConfirmingShop}
            title={`Make ${board.name} a shop board?`}
            description={`Everyone on ${selectedChannel?.name ?? 'this shop'} with access to boards will see it, and anyone who manages boards here can change, share or delete it. You stop being its owner, and its current shares go.`}
            confirmLabel="Make it a shop board"
            onConfirm={async () => {
              await makeShop({ variables: { channelId, id: board.id } })
            }}
          />
        </>
      )}
    </DetailPageLayout>
  )
}

const DRAFT_DELAY_MS = 1000

// The kept draft is the page's own staging, as it was written - anything else reads as no draft
function readDraft(payload: unknown): { name: string; reports: DraftReport[] } | null {
  const p = payload as { name?: unknown; reports?: unknown } | null | undefined
  if (!p || typeof p.name !== 'string' || !Array.isArray(p.reports)) return null
  return { name: p.name, reports: p.reports as DraftReport[] }
}

// Who sees the board, in one line under its name
export function sharingSummary(b: BoardFullFragment, shop: string): string {
  if (b.status === 'draft') return 'A draft only you can see - Save to keep it.'
  if (!b.owner) return `A shop board: everyone on ${shop} with access to boards sees it.`
  const owner = b.owner.removed ? 'Its owner was removed - someone who manages boards here can claim it.' : b.access === 'own' ? '' : `${b.owner.name}'s board. `
  if (b.owner.removed) return owner
  if (b.access === 'own' && b.shares.length === 0) return 'Private - only you can see it.'
  if (b.access === 'own') return `Shared with ${b.shares.map((s) => `${s.name} (${s.level === 'edit' ? 'can edit' : 'can view'})`).join(', ')}.`
  return `${owner}You can ${b.access === 'edit' ? 'edit' : 'view'} it.`
}

type TileProps = {
  seesData: boolean
  shop: string
  r: DraftReport
  channelId: string
  sinceDays: number
  canManage: boolean
  editing: boolean
  focused: boolean
  onEdit: () => void
  onRemove: () => void
  onDuplicate: () => void
}

function Tile({ r, channelId, sinceDays, canManage, editing, focused, onEdit, onRemove, onDuplicate, seesData, shop }: TileProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: r.key, disabled: !canManage || focused })
  const size = focused ? 'full' : (r.definition.size ?? 'medium')
  return (
    <section
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      aria-label={r.title}
      className={cn('min-w-0 rounded-md border bg-background p-4', SPAN[size], editing && 'border-foreground/40 ring-1 ring-foreground/20', isDragging && 'z-10 opacity-80')}
    >
      <div className="mb-3 flex items-start gap-1">
        {canManage && !focused && (
          <button type="button" aria-label={`Move ${r.title}`} className="-ml-1 mt-0.5 cursor-grab rounded p-0.5 text-muted-foreground hover:bg-muted" {...attributes} {...listeners}>
            <GripVerticalIcon className="size-4" />
          </button>
        )}
        <div className="min-w-0 flex-1">
          <h3 className="truncate font-medium">{r.title || 'Untitled report'}</h3>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {REPORT_KIND_LABEL[r.definition.kind] ?? r.definition.kind} · {describeReport(r.definition)}
          </p>
        </div>
        {canManage && (
          <div className="flex shrink-0">
            <Button size="icon" variant="ghost" className="size-7" aria-label={`Edit ${r.title}`} onClick={onEdit}>
              <PencilIcon className="size-3.5" />
            </Button>
            {!focused && (
              <Button size="icon" variant="ghost" className="size-7" aria-label={`Duplicate ${r.title}`} onClick={onDuplicate}>
                <CopyIcon className="size-3.5" />
              </Button>
            )}
            <Button size="icon" variant="ghost" className="size-7" aria-label={`Remove ${r.title}`} onClick={onRemove}>
              <XIcon className="size-3.5" />
            </Button>
          </div>
        )}
      </div>
      {seesData ? (
        <ReportResult channelId={channelId} d={r.definition} sinceDays={r.definition.sinceDays || sinceDays} compact={size !== 'full'} />
      ) : (
        <p className="py-6 text-center text-sm text-muted-foreground">You need customer analytics on {shop} to see this report's numbers.</p>
      )}
    </section>
  )
}

// EditPanel builds the tile being edited, beside the grid rather than over it, so the board stays usable and the tile
// shows each change as it's made - the changes are staged on the board like any other edit
function EditPanel({ r, channelId, focused, onChange, onFocus, onClose }: { r: DraftReport; channelId: string; focused: boolean; onChange: (patch: Partial<BoardReport>) => void; onFocus: () => void; onClose: () => void }) {
  const d = r.definition
  const builder = d.kind === 'stickiness' ? 'retention' : d.kind
  const setDefinition = (next: ReportDefinition) => onChange({ definition: { ...next, size: d.size, sinceDays: d.sinceDays } })
  return (
    <aside aria-label={`Edit ${r.title}`} className="sticky top-0 max-h-[calc(100vh-10rem)] w-[26rem] shrink-0 space-y-4 overflow-y-auto rounded-md border bg-background p-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="font-semibold">Edit report</h2>
        <div className="flex">
          <Button size="icon" variant="ghost" className="size-7" aria-label={focused ? 'Back to the board' : 'Expand the report'} onClick={onFocus}>
            {focused ? <Minimize2Icon className="size-4" /> : <Maximize2Icon className="size-4" />}
          </Button>
          <Button size="icon" variant="ghost" className="size-7" aria-label="Close the editor" onClick={onClose}>
            <XIcon className="size-4" />
          </Button>
        </div>
      </div>
      <div className="space-y-1">
        <Label htmlFor="tile-title">Title</Label>
        <Input id="tile-title" value={r.title} onChange={(e) => onChange({ title: e.target.value })} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1">
          <Label htmlFor="tile-kind">Report</Label>
          <select
            id="tile-kind"
            value={builder}
            onChange={(e) => {
              const next = DEFAULT_DEFINITIONS[e.target.value as ReportKind]
              onChange({ title: suggestTitle(next), definition: { ...next, size: d.size } })
            }}
            className="w-full rounded border bg-background px-2 py-1.5 text-sm"
          >
            {BUILDERS.map((b) => (
              <option key={b.kind} value={b.kind}>
                {b.label}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label htmlFor="tile-size">Size</Label>
          <select id="tile-size" value={d.size ?? 'medium'} onChange={(e) => onChange({ definition: { ...d, size: e.target.value as ReportSize } })} className="w-full rounded border bg-background px-2 py-1.5 text-sm">
            {(Object.keys(REPORT_SIZE_LABEL) as ReportSize[]).map((s) => (
              <option key={s} value={s}>
                {REPORT_SIZE_LABEL[s]}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="border-t pt-4">
        <ReportControls channelId={channelId} value={d} onChange={setDefinition} />
      </div>
      <p className="text-xs text-muted-foreground">Changes show on the tile straight away and are kept when you Save the board. Esc closes this panel.</p>
    </aside>
  )
}
