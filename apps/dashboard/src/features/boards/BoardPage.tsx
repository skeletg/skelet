import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, rectSortingStrategy, sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { CopyIcon, GripVerticalIcon, PencilIcon, PlusIcon, Share2Icon, StarIcon, Trash2Icon, XIcon } from 'lucide-react'
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
import { BUILDERS, KINDS, ReportResult, describeReport, kindOf } from '@/shared/analytics/kinds'
import type { ReportKind } from '@/shared/analytics/kinds/definition'
import { cleanReport, type ReportSize } from './report'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import {
  BoardDocument,
  BoardsDocument,
  DeleteBoardDocument,
  DiscardBoardDraftDocument,
  SaveBoardDocument,
  SaveBoardDraftDocument,
  SetBoardFavouriteDocument,
  type BoardFullFragment,
} from './Boards.gql'
import { ACCESS_LABEL, can, verb } from './access'
import { AccessRequestList, RequestAccess, notOpenable } from './AccessRequests'
import { HandOverDialog } from './HandOverDialog'
import { ShareDialog } from './ShareDialog'
import { boardDiff, fresh, keyed, unkeyed, type AddToBoardState, type DraftReport } from './boardDiff'
import { EditPanel } from './EditPanel'

// A tile's width in the board's grid, which is 1 column narrow, 2 from a medium width, 4 wide and 5 very wide - measured
// on the grid itself, so opening the edit panel narrows it rather than squeezing four tiles into the space left
const SPAN: Record<ReportSize, string> = { SMALL: 'col-span-1', MEDIUM: 'col-span-1 @2xl:col-span-2', FULL: 'col-span-full' }

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
  const saved = useMemo(() => (board?.reports ?? []).map(cleanReport), [board])
  const canEdit = can(board?.access, 'EDIT')
  const canOwn = can(board?.access, 'OWN')

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
    // A kept draft's positions are the board as it was then - one past the end of the board as it is now reads as added,
    // and Save refuses the draft as stale anyway
    const reports = kept ? kept.reports.map((r) => (r.from !== null && r.from >= saved.length ? { ...r, from: null } : r)) : keyed(saved)
    if (arriving) {
      const added = fresh(arriving)
      setDraft([...reports, added])
      setEditing(added.key)
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
  const [sharing, setSharing] = useState(false)
  const [handingOver, setHandingOver] = useState(false)
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

  useSaveSection(
    {
      id: 'board',
      title: 'Board',
      diff: changes,
      save: async () => {
        if (!board) return
        const res = await saveBoard({
          variables: { channelId, id: board.id, name: draftName.trim(), reports: unkeyed(draft).map((r) => ({ ...r, title: r.title.trim() })), readAt: base },
        })
        const next = res.data?.saveBoard
        if (next) {
          setDraft(keyed(next.reports.map(cleanReport)))
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

  const update = (key: string, patch: Partial<DraftReport>) => setDraft((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))
  const add = (kind: ReportKind) => {
    const { start, title } = KINDS[kind]
    const added = fresh({ title: title(start), size: 'MEDIUM', definition: start })
    setDraft((rs) => [...rs, added])
    setEditing(added.key)
    setFocused(false)
  }
  const remove = (key: string) => {
    setDraft((rs) => rs.filter((r) => r.key !== key))
    if (editing === key) setEditing(null)
  }
  const duplicate = (key: string) =>
    setDraft((rs) => {
      const i = rs.findIndex((r) => r.key === key)
      return [...rs.slice(0, i + 1), fresh({ ...rs[i], title: `${rs[i].title} (copy)` }), ...rs.slice(i + 1)]
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
    setDraft(keyed(saved))
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
        backTo="/synapse/boards"
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
            {canOwn && board?.owner && (
              <Button size="sm" variant="outline" onClick={() => setSharing(true)}>
                <Share2Icon className="size-4" />
                Share
              </Button>
            )}
            {canOwn && board && (
              <Button size="sm" variant="ghost" onClick={() => setHandingOver(true)}>
                Hand over
              </Button>
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
        {notOpenable(error) && <RequestAccess channelId={channelId} id={id} />}
        {error && !notOpenable(error) && (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load the board</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        )}
        {loading && !board && <p className="py-6 text-sm text-muted-foreground">Loading…</p>}
        {board && canOwn && <AccessRequestList channelId={channelId} board={board} />}
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
          navigate('/synapse/boards')
        }}
      />
      {board && (
        <>
          {sharing && <ShareDialog channelId={channelId} shop={selectedChannel?.name ?? 'this shop'} board={board} onClose={() => setSharing(false)} />}
          <HandOverDialog channelId={channelId} shop={selectedChannel?.name ?? 'this shop'} board={board} canManage={canManage} open={handingOver} onOpenChange={setHandingOver} />
        </>
      )}
    </DetailPageLayout>
  )
}

const DRAFT_DELAY_MS = 1000

// The kept draft is the page's own staging, as it was written - anything else, such as one kept by an older build,
// reads as no draft
function readDraft(payload: unknown): { name: string; reports: DraftReport[] } | null {
  const p = payload as { name?: unknown; reports?: unknown } | null | undefined
  if (!p || typeof p.name !== 'string' || !Array.isArray(p.reports)) return null
  const current = (r: unknown) => typeof r === 'object' && r !== null && 'from' in r && 'size' in r && 'definition' in r
  return p.reports.every(current) ? { name: p.name, reports: p.reports as DraftReport[] } : null
}

// Who sees the board, in one line under its name
export function sharingSummary(b: BoardFullFragment, shop: string): string {
  if (!b.owner) return `A shop board: everyone on ${shop} with access to boards sees it.`
  if (b.owner.removed) return 'Its owner was removed - a superuser can hand it to someone.'
  if (b.access !== 'OWN') return `${b.owner.name}'s board. You can ${verb(b.access)} it.`
  if (b.shares.length === 0) return 'Private - only you can see it.'
  return `Shared with ${b.shares.map((s) => `${s.name} (${ACCESS_LABEL[s.level].toLowerCase()})`).join(', ')}.`
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
  const size: ReportSize = focused ? 'FULL' : r.size
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
            {kindOf(r.definition)?.label ?? r.definition.kind} · {describeReport(r.definition)}
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
        <ReportResult channelId={channelId} d={r.definition} sinceDays={r.definition.sinceDays || sinceDays} compact={size !== 'FULL'} />
      ) : (
        <p className="py-6 text-center text-sm text-muted-foreground">You need customer analytics on {shop} to see this report's numbers.</p>
      )}
    </section>
  )
}
