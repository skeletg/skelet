import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { LayoutGridIcon, PlusIcon } from 'lucide-react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfigScope } from '@/lib/config-scope'
import { formatNumber } from '@/lib/format'
import { usePermissions } from '@/shared/auth/usePermissions'
import { BoardsDocument, CreateBoardDocument, CreateStarterBoardsDocument, type BoardsQuery } from './Boards.gql'

type Row = BoardsQuery['boards'][number]

export function BoardsPage() {
  const urlState = useListUrlState()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { canAccessChannelScope } = usePermissions()
  const canManage = isChannel && canAccessChannelScope('boards.manage', channelId)
  const { data, loading, error } = useQuery(BoardsDocument, { variables: { channelId }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const [createStarters, starters] = useMutation(CreateStarterBoardsDocument, { refetchQueries: [BoardsDocument] })
  const rows = useMemo(() => data?.boards ?? [], [data])

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/boards/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      { id: 'reports', header: 'Reports', size: 100, cell: ({ row }) => <span className="text-xs tabular-nums">{formatNumber(row.original.reportCount)}</span> },
      {
        id: 'updated',
        header: 'Last saved',
        size: 180,
        cell: ({ row }) => <span className="text-xs text-muted-foreground">{new Date(row.original.updatedAt).toLocaleString()}</span>,
      },
    ],
    [],
  )

  if (!isChannel) {
    return (
      <Alert>
        <AlertTitle>Select a channel</AlertTitle>
        <AlertDescription>Boards belong to a shop - pick one in the header scope picker.</AlertDescription>
      </Alert>
    )
  }

  return (
    <DataTable
      urlState={urlState}
      columns={columns}
      data={rows}
      loading={loading && !data}
      error={error ? { message: error.message } : null}
      getRowId={(row) => row.id}
      emptyState={
        <div className="space-y-3 py-6 text-center">
          <p>No boards yet. A board puts the reports you watch side by side, shared with everyone on this shop.</p>
          {canManage && (
            <Button size="sm" variant="outline" disabled={starters.loading} onClick={() => void createStarters({ variables: { channelId } })}>
              <LayoutGridIcon className="size-4" />
              Add the starter boards
            </Button>
          )}
          {starters.error && <p className="text-sm text-destructive">{starters.error.message}</p>}
        </div>
      }
      searchPlaceholder="Search boards…"
      toolbarRightSlot={canManage ? <NewBoardButton channelId={channelId} /> : null}
    />
  )
}

function NewBoardButton({ channelId }: { channelId: string }) {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [create, { loading, error }] = useMutation(CreateBoardDocument, { refetchQueries: [BoardsDocument] })
  async function submit() {
    const res = await create({ variables: { channelId, name: name.trim() } })
    const id = res.data?.createBoard.id
    if (id) {
      setOpen(false)
      navigate(`/boards/${id}`)
    }
  }
  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        <PlusIcon className="size-4" />
        New board
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>New board</DialogTitle>
            <DialogDescription>Add reports to it from Insights, Funnel, Retention, Engagement and Flow.</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault()
              if (name.trim()) void submit().catch(() => {})
            }}
          >
            <Label htmlFor="board-name">Name</Label>
            <Input id="board-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Weekly review" autoFocus />
            {error && <p className="text-sm text-destructive">{error.message}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={!name.trim() || loading}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
