import { useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { LayoutGridIcon, PlusIcon, StarIcon } from 'lucide-react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useConfigScope } from '@/lib/config-scope'
import { formatNumber } from '@/lib/format'
import { usePermissions } from '@/shared/auth/usePermissions'
import { MeDocument } from '@/shared/auth/Me.gql'
import { ChannelRequired } from '@/shared/analytics/ChannelRequired'
import { BoardsDocument, CreateBoardDocument, CreateStarterBoardsDocument, SetBoardFavouriteDocument, type BoardsQuery } from './Boards.gql'

type Row = BoardsQuery['boards'][number]

// The list's views, each a filter over the boards the caller can open; "every" is a superuser's whole shop
export const BOARD_VIEWS = [
  { v: 'all', l: 'All I can see' },
  { v: 'favourites', l: 'Favourites' },
  { v: 'mine', l: 'Mine' },
  { v: 'shared', l: 'Shared with me' },
  { v: 'shop', l: 'Shop boards' },
  { v: 'drafts', l: 'My drafts' },
] as const
type View = (typeof BOARD_VIEWS)[number]['v'] | 'every'

export function inView(view: View, me: string, b: Row): boolean {
  switch (view) {
    case 'favourites':
      return b.favourite
    case 'mine':
      return b.owner?.id === me
    case 'shared':
      return !!b.owner && b.owner.id !== me
    case 'shop':
      return !b.owner
    case 'drafts':
      return b.status === 'draft' || !!b.draftUpdatedAt
    default:
      return true
  }
}

const ACCESS_LABEL: Record<string, string> = { view: 'Can view', edit: 'Can edit', own: 'Owner' }

export function BoardsPage() {
  const urlState = useListUrlState()
  const [params, setParams] = useSearchParams()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const channelId = effectiveChannelId ?? ''
  const { canAccessChannelScope, isSuperuser } = usePermissions()
  const { data: meData } = useQuery(MeDocument, { fetchPolicy: 'cache-first' })
  const me = meData?.me?.id ?? ''
  const canManage = isChannel && canAccessChannelScope('boards.manage', channelId)
  const view = ((params.get('view') ?? 'all') as View)
  const every = view === 'every' && isSuperuser
  const { data, loading, error } = useQuery(BoardsDocument, { variables: { channelId, all: every }, skip: !isChannel, fetchPolicy: 'cache-and-network' })
  const [createStarters, starters] = useMutation(CreateStarterBoardsDocument, { refetchQueries: [BoardsDocument] })
  // A star is a personal bookmark, like a nav pin - it applies at once, with nothing to stage
  const [star] = useMutation(SetBoardFavouriteDocument, { refetchQueries: [BoardsDocument] })
  const rows = useMemo(() => (data?.boards ?? []).filter((b) => every || inView(view, me, b)), [data, view, me, every])
  const setView = (v: string) =>
    setParams(
      (p) => {
        p.set('view', v)
        return p
      },
      { replace: true },
    )

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        id: 'star',
        size: 40,
        header: () => <span className="sr-only">Favourite</span>,
        cell: ({ row }) => (
          <button
            type="button"
            aria-pressed={row.original.favourite}
            aria-label={row.original.favourite ? `Unstar ${row.original.name}` : `Star ${row.original.name}`}
            onClick={() => void star({ variables: { channelId, id: row.original.id, favourite: !row.original.favourite } })}
            className="rounded p-1 text-muted-foreground hover:text-foreground"
          >
            <StarIcon className={row.original.favourite ? 'size-4 fill-current text-foreground' : 'size-4'} />
          </button>
        ),
      },
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <Link to={`/boards/${row.original.id}`} className="font-medium hover:underline">
              {row.original.name}
            </Link>
            {row.original.status === 'draft' ? (
              <Badge variant="outline">Draft</Badge>
            ) : row.original.draftUpdatedAt ? (
              <Badge variant="outline">Unsaved changes</Badge>
            ) : null}
          </span>
        ),
      },
      {
        id: 'owner',
        header: 'Owner',
        size: 200,
        cell: ({ row }) => {
          const o = row.original.owner
          if (!o) return <span className="text-xs text-muted-foreground">Shop board</span>
          return <span className={o.removed ? 'text-xs text-warning' : 'text-xs'}>{o.id === me ? 'You' : o.name}</span>
        },
      },
      { id: 'access', header: 'You', size: 100, cell: ({ row }) => <span className="text-xs text-muted-foreground">{ACCESS_LABEL[row.original.access] ?? row.original.access}</span> },
      { id: 'reports', header: 'Reports', size: 90, cell: ({ row }) => <span className="text-xs tabular-nums">{formatNumber(row.original.reportCount)}</span> },
      {
        id: 'updated',
        header: 'Last saved',
        size: 180,
        cell: ({ row }) => <span className="text-xs text-muted-foreground">{new Date(row.original.updatedAt).toLocaleString()}</span>,
      },
    ],
    [channelId, me, star],
  )

  if (!isChannel) return <ChannelRequired what="Boards" />

  return (
    <div className="grid gap-4">
      <Tabs value={every ? 'every' : view} onValueChange={setView}>
        <TabsList>
          {BOARD_VIEWS.map((v) => (
            <TabsTrigger key={v.v} value={v.v}>
              {v.l}
            </TabsTrigger>
          ))}
          {isSuperuser && <TabsTrigger value="every">Every board</TabsTrigger>}
        </TabsList>
      </Tabs>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading && !data}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState={
          <div className="space-y-3 py-6 text-center">
            <p>{(data?.boards.length ?? 0) > 0 ? 'No boards in this view.' : 'No boards yet. A board puts the reports you watch side by side, private to you until you share it.'}</p>
            {canManage && (data?.boards.length ?? 0) === 0 && (
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
    </div>
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
            <DialogDescription>It's a draft only you can see until you save it, and private until you share it.</DialogDescription>
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
