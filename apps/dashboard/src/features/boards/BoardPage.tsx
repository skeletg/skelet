import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { ChevronDownIcon, ChevronUpIcon, Trash2Icon, XIcon } from 'lucide-react'
import { DetailPageBody, DetailPageHeader, DetailPageLayout, DetailPageSection } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { PeriodPicker } from '@/shared/analytics/PeriodPicker'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { BoardDocument, BoardsDocument, DeleteBoardDocument, SaveBoardDocument } from './Boards.gql'
import { boardDiff, type AddToBoardState } from './boardDiff'
import { cleanReport, reportOf, type BoardReport } from './report'
import { ReportTile } from './ReportTile'

export function BoardPage() {
  const { id = '' } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const location = useLocation()
  const { effectiveChannelId, isChannel, selectedChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { canAccessChannelScope } = usePermissions()
  const canManage = isChannel && canAccessChannelScope('boards.manage', channelId)
  const { data, loading, error } = useQuery(BoardDocument, { variables: { channelId, id }, skip: !isChannel || !id })
  const board = data?.board
  const saved = useMemo(() => (board?.reports ?? []).map(cleanReport), [board])

  const [draftName, setDraftName] = useState('')
  const [draft, setDraft] = useState<BoardReport[]>([])
  const [sinceDays, setSinceDays] = useState(30)
  const [loadedId, setLoadedId] = useState('') // the board the draft holds - until it's this one there's nothing to diff
  const [arriving] = useState(() => (location.state as AddToBoardState | null)?.add) // what Add to board sent, read once
  // Adopt the board on its identity only, so a refetch never wipes edits mid-way
  if (board && loadedId !== board.id) {
    setLoadedId(board.id)
    setDraftName(board.name)
    setDraft(arriving ? [...saved, arriving] : saved)
  }
  useEffect(() => {
    if (arriving) navigate(location.pathname, { replace: true, state: null }) // a refresh mustn't add it a second time
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const registry = useSaveRegistry(board ? `Board · ${board.name}` : 'Board')
  const [saveBoard] = useMutation(SaveBoardDocument, { refetchQueries: [BoardsDocument] })
  const [deleteBoard] = useMutation(DeleteBoardDocument, { refetchQueries: [BoardsDocument] })
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  useSaveSection(
    {
      id: 'board',
      title: 'Board',
      diff: board && loadedId === board.id ? boardDiff(board.name, saved, draftName, draft) : [],
      save: async () => {
        if (!board) return
        const res = await saveBoard({
          variables: { channelId, id: board.id, name: draftName.trim(), reports: draft.map((r) => ({ ...r, title: r.title.trim() })), readAt: board.updatedAt },
        })
        const next = res.data?.saveBoard
        if (next) setDraft(next.reports.map(cleanReport))
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

  const move = (i: number, d: -1 | 1) =>
    setDraft((rs) => {
      const j = i + d
      if (j < 0 || j >= rs.length) return rs
      const next = [...rs]
      ;[next[i], next[j]] = [next[j], next[i]]
      return next
    })

  if (!isChannel) {
    return (
      <Alert>
        <AlertTitle>Select a channel</AlertTitle>
        <AlertDescription>Boards belong to a shop - pick one in the header scope picker.</AlertDescription>
      </Alert>
    )
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/boards"
        backLabel="Back to boards"
        title={board?.name ?? 'Board'}
        badges={<Badge variant="secondary">{selectedChannel?.name}</Badge>}
        meta={<span className="text-xs text-muted-foreground">Shared with everyone on this shop. A report without its own period uses the board's.</span>}
        actions={
          <div className="flex items-center gap-3">
            <PeriodPicker value={sinceDays} onChange={setSinceDays} />
            {canManage && board && (
              <>
                <Button size="sm" variant="outline" onClick={() => setConfirmingDelete(true)}>
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
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
        {board && canManage && (
          <DetailPageSection title="Name">
            <div className="max-w-md">
              <Label htmlFor="board-name" className="sr-only">
                Board name
              </Label>
              <Input id="board-name" value={draftName} onChange={(e) => setDraftName(e.target.value)} />
            </div>
          </DetailPageSection>
        )}
        {board && draft.length === 0 && (
          <DetailPageSection>
            <p className="py-6 text-center text-sm text-muted-foreground">
              No reports yet. Open Insights, Funnel, Retention, Engagement or Flow and use Add to board.
            </p>
          </DetailPageSection>
        )}
        {board && draft.length > 0 && (
          <div className="grid gap-4 md:grid-cols-2">
            {draft.map((r, i) => (
              <section key={`${i}-${r.title}`} className="rounded-md border p-4" aria-label={r.title}>
                <div className="mb-3 flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    {canManage ? (
                      <Input
                        aria-label={`Title of report ${i + 1}`}
                        value={r.title}
                        onChange={(e) => setDraft((rs) => rs.map((x, k) => (k === i ? { ...x, title: e.target.value } : x)))}
                        className="h-8 font-medium"
                      />
                    ) : (
                      <h3 className="font-medium">{r.title}</h3>
                    )}
                    <p className="mt-1 truncate text-xs text-muted-foreground">
                      {reportOf(r.definition)?.label ?? r.definition.kind} · {reportOf(r.definition)?.describe(r.definition)}
                    </p>
                  </div>
                  {canManage && (
                    <div className="flex shrink-0">
                      <Button size="icon" variant="ghost" aria-label={`Move ${r.title} up`} disabled={i === 0} onClick={() => move(i, -1)}>
                        <ChevronUpIcon className="size-4" />
                      </Button>
                      <Button size="icon" variant="ghost" aria-label={`Move ${r.title} down`} disabled={i === draft.length - 1} onClick={() => move(i, 1)}>
                        <ChevronDownIcon className="size-4" />
                      </Button>
                      <Button size="icon" variant="ghost" aria-label={`Remove ${r.title}`} onClick={() => setDraft((rs) => rs.filter((_, k) => k !== i))}>
                        <XIcon className="size-4" />
                      </Button>
                    </div>
                  )}
                </div>
                <ReportTile channelId={channelId} definition={r.definition} sinceDays={sinceDays} />
              </section>
            ))}
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
        description="Its reports go with it, for everyone on this shop. The data they show stays."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!board) return
          await deleteBoard({ variables: { channelId, id: board.id } })
          navigate('/boards')
        }}
      />
    </DetailPageLayout>
  )
}
