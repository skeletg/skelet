import { useMutation } from '@apollo/client/react'
import { CombinedGraphQLErrors } from '@apollo/client/errors'
import { Button } from '@/components/ui/button'
import { BoardDocument, BoardsDocument, DismissBoardAccessRequestDocument, RequestBoardAccessDocument, SetBoardSharesDocument, type BoardFullFragment } from './Boards.gql'

// Signals answers not-found alike for a board that isn't there and one the admin can't open, so an id says nothing
export const notOpenable = (error: unknown) =>
  CombinedGraphQLErrors.is(error) && error.errors.some((e) => e.extensions?.code === 'SIGNALS_NOT_FOUND')

// RequestAccess is what someone sees on a board they can't open: a way to ask its owner. The answer is the same
// whether or not the board exists, so asking tells them nothing either
export function RequestAccess({ channelId, id }: { channelId: string; id: string }) {
  const [ask, { called, loading, error }] = useMutation(RequestBoardAccessDocument)
  return (
    <div className="mx-6 my-6 space-y-3 rounded-md border p-4 text-sm">
      <p className="font-medium">You can't open this board</p>
      <p className="text-muted-foreground">It hasn't been shared with you, or it isn't on this shop. If someone sent you the link, ask its owner for access.</p>
      {called && !loading && !error ? (
        <p className="text-success">Asked. If the board is there, its owner sees your request on it.</p>
      ) : (
        <div className="flex gap-2">
          <Button size="sm" variant="outline" disabled={loading} onClick={() => void ask({ variables: { channelId, id, level: 'view' } }).catch(() => {})}>
            Ask to view it
          </Button>
          <Button size="sm" variant="outline" disabled={loading} onClick={() => void ask({ variables: { channelId, id, level: 'edit' } }).catch(() => {})}>
            Ask to edit it
          </Button>
        </div>
      )}
      {error && <p className="text-destructive">{error.message}</p>}
    </div>
  )
}

// AccessRequestList shows the owner who asked for access. Grant shares the board at the level asked for and Dismiss
// turns it down - each an explicit action of its own, like the share dialog's, so neither is staged on the page
export function AccessRequestList({ channelId, board }: { channelId: string; board: BoardFullFragment }) {
  const refetchQueries = [BoardDocument, BoardsDocument]
  const [share, sharing] = useMutation(SetBoardSharesDocument, { refetchQueries })
  const [dismiss, dismissing] = useMutation(DismissBoardAccessRequestDocument, { refetchQueries })
  if (board.accessRequests.length === 0) return null
  const busy = sharing.loading || dismissing.loading
  const grant = (adminId: string, level: string) => {
    const shares = board.shares.map(({ adminId, roleId, level }) => ({ adminId, roleId, level }))
    return share({ variables: { channelId, id: board.id, shares: [...shares.filter((s) => s.adminId !== adminId), { adminId, level }] } })
  }
  return (
    <div className="mx-6 mt-4 rounded-md border border-info/40 bg-info-soft text-sm">
      <p className="px-3 pt-2 font-medium">Asked for access</p>
      <ul className="divide-y">
        {board.accessRequests.map((r) => (
          <li key={r.adminId} className="flex flex-wrap items-center gap-3 px-3 py-2">
            <span className="flex-1">
              {r.name} wants to {r.level === 'edit' ? 'edit' : 'view'} it <span className="text-xs text-muted-foreground">· {new Date(r.requestedAt).toLocaleString()}</span>
            </span>
            <Button size="sm" variant="outline" disabled={busy} onClick={() => void grant(r.adminId, r.level).catch(() => {})}>
              Let them {r.level === 'edit' ? 'edit' : 'view'}
            </Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => void dismiss({ variables: { channelId, id: board.id, adminId: r.adminId } }).catch(() => {})}>
              Dismiss
            </Button>
          </li>
        ))}
      </ul>
      {(sharing.error ?? dismissing.error) && <p className="px-3 pb-2 text-destructive">{(sharing.error ?? dismissing.error)?.message}</p>}
    </div>
  )
}
