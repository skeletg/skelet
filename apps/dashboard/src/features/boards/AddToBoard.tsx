import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { LayoutGridIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePermissions } from '@/shared/auth/usePermissions'
import { cleanDefinition, describeReport, type ReportDefinition } from '@/shared/analytics/report'
import { BoardsDocument, CreateBoardDocument } from './Boards.gql'
import type { AddToBoardState } from './boardDiff'

// AddToBoard puts the report on screen onto a board. An existing board opens with it staged, so it still goes
// through that board's Save; a new board is created with it, which is the dialog's own explicit action
export function AddToBoard({ channelId, title, definition }: { channelId: string; title: string; definition: ReportDefinition }) {
  const navigate = useNavigate()
  const { canAccessChannelScope } = usePermissions()
  const canManage = canAccessChannelScope('boards.manage', channelId)
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(title)
  const [boardId, setBoardId] = useState('')
  const [newBoard, setNewBoard] = useState('')
  const { data } = useQuery(BoardsDocument, { variables: { channelId }, skip: !open })
  const [create, { loading, error }] = useMutation(CreateBoardDocument, { refetchQueries: [BoardsDocument] })
  if (!canManage) return null
  const report = { title: name.trim() || title, definition: cleanDefinition(definition) }

  async function submit() {
    if (boardId) {
      navigate(`/boards/${boardId}`, { state: { add: report } satisfies AddToBoardState })
      return
    }
    const res = await create({ variables: { channelId, name: newBoard.trim(), reports: [report] } })
    const id = res.data?.createBoard.id
    if (id) navigate(`/boards/${id}`)
  }

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={() => {
          setName(title)
          setOpen(true)
        }}
      >
        <LayoutGridIcon className="size-4" />
        Add to board
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Add to board</DialogTitle>
            <DialogDescription>{describeReport(report.definition)}</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault()
              void submit().catch(() => {})
            }}
          >
            <div className="space-y-1">
              <Label htmlFor="report-title">Report title</Label>
              <Input id="report-title" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label htmlFor="report-board">Board</Label>
              <select
                id="report-board"
                value={boardId}
                onChange={(e) => setBoardId(e.target.value)}
                className="w-full rounded border bg-background px-2 py-1.5 text-sm"
              >
                <option value="">A new board…</option>
                {(data?.boards ?? []).map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.name}
                  </option>
                ))}
              </select>
            </div>
            {!boardId && (
              <div className="space-y-1">
                <Label htmlFor="report-new-board">New board name</Label>
                <Input id="report-new-board" value={newBoard} onChange={(e) => setNewBoard(e.target.value)} placeholder="e.g. Weekly review" />
              </div>
            )}
            {boardId && <p className="text-xs text-muted-foreground">The board opens with the report added - Save there to keep it.</p>}
            {error && <p className="text-sm text-destructive">{error.message}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={loading || (!boardId && !newBoard.trim())}>
                {boardId ? 'Open board' : 'Create board'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
