import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { CreateSegmentDocument, DeleteSegmentDocument, SegmentsPageDocument } from './Segments.gql'
import { segmentType, summarizeRules, type Rules } from './rules'
import { usePermissions } from '@/shared/auth/usePermissions'

type SegmentRow = {
  id: string
  slug: string
  name: string
  description: string
  isActive: boolean
  memberCount: number
  rules: unknown
  rulesProblem?: string | null
  updatedAt: string
}

export function SegmentsListPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  // Creating and deleting segments need promotions.manage - a viewer's buttons only led to a refused save
  const canManage = usePermissions().has('promotions.manage')
  const navigate = useNavigate()
  const { data, loading, error, refetch } = useQuery(SegmentsPageDocument, {
    variables: { limit: 200, offset: 0 },
  })
  const rows = useMemo(() => (data?.customerSegments.items ?? []) as SegmentRow[], [data])

  const [createOpen, setCreateOpen] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState<SegmentRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteSegment, { loading: deleting }] = useMutation(DeleteSegmentDocument)

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteSegment({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
      void refetch()
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  const columns = useMemo<ColumnDef<SegmentRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="min-w-0">
            <Link to={`/segments/${row.original.id}`} className="font-medium hover:underline">
              {row.original.name}
            </Link>
            <p className="max-w-md truncate text-xs text-muted-foreground">
              {summarizeRules(row.original.rules as Rules | null)}
            </p>
          </div>
        ),
      },
      {
        accessorKey: 'slug',
        header: 'Slug',
        cell: ({ row }) => <code className="font-mono text-xs">{row.original.slug}</code>,
      },
      {
        id: 'type',
        header: 'Type',
        size: 90,
        cell: ({ row }) => {
          const kind = segmentType(row.original.rules, row.original.rulesProblem)
          return kind === 'needs-attention' ? (
            <Badge variant="destructive" title={row.original.rulesProblem ?? undefined}>
              Needs attention
            </Badge>
          ) : (
            <Badge variant="outline">{kind === 'dynamic' ? 'Dynamic' : 'Manual'}</Badge>
          )
        },
      },
      {
        accessorKey: 'memberCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Members" />,
        size: 100,
      },
      {
        accessorKey: 'isActive',
        header: 'Status',
        size: 100,
        cell: ({ row }) =>
          row.original.isActive ? (
            <span className="inline-flex items-center gap-1.5 text-xs">
              <span className="size-1.5 rounded-full bg-success" /> Active
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
              <span className="size-1.5 rounded-full bg-muted-foreground/40" /> Inactive
            </span>
          ),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {new Date(row.original.updatedAt).toLocaleDateString()}
          </span>
        ),
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label={`Actions for ${row.original.name}`}
              >
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/segments/${row.original.id}`}>Open</Link>
              </DropdownMenuItem>
              {canManage && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setDeleteError(null)
                      setConfirmingDelete(row.original)
                    }}
                  >
                    <Trash2Icon className="size-4" />
                    Delete
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [canManage],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Segments"
        meta="Customer buckets used by promotions, campaign audiences, flow triggers and audience sync. Dynamic segments recompute from rules every 6 hours; manual ones hold whoever you assign."
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable
            urlState={urlState}
            columns={columns}
            data={rows}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(row) => row.id}
            emptyState="No segments yet. Create one, then give it rules to compute membership automatically."
            searchPlaceholder="Search name or slug…"
            initialSorting={[{ id: 'name', desc: false }]}
            toolbarRightSlot={
              canManage && (
                <Button size="sm" onClick={() => setCreateOpen(true)}>
                  <PlusIcon className="size-4" />
                  New segment
                </Button>
              )
            }
          />
        </DetailPageSection>
      </DetailPageBody>

      <CreateSegmentDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(id) => navigate(`/segments/${id}`)}
      />

      <AlertDialog
        open={!!confirmingDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingDelete(null)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {confirmingDelete?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Membership is deleted with it. Promotions, campaigns and flows referencing the slug
              "{confirmingDelete?.slug}" stop matching anyone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                onConfirmDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete segment'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}

function CreateSegmentDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (id: string) => void
}) {
  const [create, { loading }] = useMutation(CreateSegmentDocument)
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)

  const [prevOpen, setPrevOpen] = useState(open)
  if (prevOpen !== open) {
    setPrevOpen(open)
    if (open) {
      setName('')
      setSlug('')
      setDescription('')
      setError(null)
    }
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      const { data } = await create({
        variables: { input: { slug: slug.trim(), name: name.trim(), description: description.trim() || undefined } },
      })
      onOpenChange(false)
      if (data) onCreated(data.createCustomerSegment.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={onSubmit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>New segment</DialogTitle>
            <DialogDescription>
              Starts as a manual segment - add rules on the detail page to make it dynamic.
            </DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-2">
              <Label htmlFor="new-seg-name">Name</Label>
              <Input
                id="new-seg-name"
                value={name}
                required
                onChange={(e) => {
                  setName(e.target.value)
                  if (!slug || slug === toSlug(name)) setSlug(toSlug(e.target.value))
                }}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="new-seg-slug">Slug</Label>
              <Input
                id="new-seg-slug"
                value={slug}
                required
                onChange={(e) => setSlug(e.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="new-seg-desc">Description</Label>
            <Textarea
              id="new-seg-desc"
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          {error && (
            <Alert variant="destructive">
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading}>
              {loading ? 'Creating…' : 'Create segment'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const toSlug = (s: string) =>
  s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
