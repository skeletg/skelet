import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { BoardDocument, BoardShareTargetsDocument, BoardsDocument, TransferBoardDocument, type BoardFullFragment } from './Boards.gql'

const SHOP = 'shop' // the picker's value for "nobody - a shop board"

// HandOverDialog gives the board to another admin who can open boards here, or - for someone who manages boards -
// makes it a shop board. An explicit action of its own, so it isn't staged on the page (conventions.md). The board is
// read again after: if the giver can't open it any more, the page says so the way it does for any board they can't
export function HandOverDialog({ channelId, shop, board, canManage, onClose }: { channelId: string; shop: string; board: BoardFullFragment; canManage: boolean; onClose: () => void }) {
  const [to, setTo] = useState('')
  const { data } = useQuery(BoardShareTargetsDocument, { variables: { channelId } })
  const [transfer, { loading, error }] = useMutation(TransferBoardDocument, { refetchQueries: [BoardsDocument, BoardDocument], awaitRefetchQueries: true })
  const admins = (data?.boardShareTargets ?? []).filter((t) => t.kind === 'admin' && t.seesBoards && t.id !== board.owner?.id)

  async function submit() {
    await transfer({ variables: { channelId, id: board.id, ownerId: to === SHOP ? null : to } })
    onClose()
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Hand over {board.name}</DialogTitle>
          <DialogDescription>The new owner decides who it's shared with. Its shares stay as they are.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (to) void submit().catch(() => {})
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="board-new-owner">New owner</Label>
            <select id="board-new-owner" value={to} onChange={(e) => setTo(e.target.value)} className="w-full rounded border bg-background px-2 py-1.5 text-sm">
              <option value="">Pick an admin…</option>
              {admins.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
              {canManage && board.owner && <option value={SHOP}>Nobody - make it a shop board</option>}
            </select>
          </div>
          {to === SHOP && <p className="text-xs text-muted-foreground">Everyone on {shop} with access to boards will see it, anyone who manages boards here runs it, and its shares go.</p>}
          {error && <p className="text-sm text-destructive">{error.message}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!to || loading}>
              Hand over
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
