import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleOffIcon,
  ClockIcon,
  ArchiveIcon,
  MoreHorizontalIcon,
  PlusIcon,
} from 'lucide-react'
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import {
  BlogAuthorsDocument,
  BlogPostsDocument,
  DeleteBlogPostDocument,
  type BlogPostRowFragment,
} from './Blog.gql'

function postTitle(p: BlogPostRowFragment) {
  return (
    p.content?.title?.trim() ||
    p.allContent.find((c) => c.title.trim())?.title.trim() ||
    p.slug
  )
}

function statusBadge(status: BlogPostRowFragment['status']) {
  switch (status) {
    case 'PUBLISHED':
      return (
        <Badge variant="secondary" className="gap-1">
          <CheckCircle2Icon className="size-3" /> Published
        </Badge>
      )
    case 'SCHEDULED':
      return (
        <Badge variant="outline" className="gap-1">
          <ClockIcon className="size-3" /> Scheduled
        </Badge>
      )
    case 'ARCHIVED':
      return (
        <Badge variant="outline" className="gap-1 text-rose-500">
          <ArchiveIcon className="size-3" /> Archived
        </Badge>
      )
    default:
      return (
        <Badge variant="outline" className="gap-1 text-muted-foreground">
          <CircleOffIcon className="size-3" /> Draft
        </Badge>
      )
  }
}

export function BlogPostsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  // Scope (channel + locale) rides request headers - see lib/apollo.ts - so neither is a query var - In CHANNEL mode
  // the list is restricted to that store's posts; in GLOBAL mode every post is shown
  const variables = useMemo(
    () => ({
      filter: { includeUnpublished: true },
      limit: 500,
      offset: 0,
    }),
    [],
  )

  const { data, loading, error } = useQuery(BlogPostsDocument, { variables })
  const { data: storesData } = useQuery(ChannelsDocument)
  const { data: authorsData } = useQuery(BlogAuthorsDocument)
  const stores = useMemo(() => storesData?.channels ?? [], [storesData])
  const authors = authorsData?.blogAuthors ?? []

  const [deletePost, { loading: deleting }] = useMutation(DeleteBlogPostDocument, {
    refetchQueries: [{ query: BlogPostsDocument, variables }],
  })
  const [confirmingDelete, setConfirmingDelete] = useState<BlogPostRowFragment | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const rows = data?.blogPosts.items ?? []

  const columns = useMemo<ColumnDef<BlogPostRowFragment, unknown>[]>(
    () => [
      {
        id: 'title',
        accessorFn: (row) => postTitle(row),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
        cell: ({ row }) => {
          const p = row.original
          return (
            <Link to={`/blog/posts/${p.id}`} className="grid gap-0.5">
              <span className="font-medium hover:underline">{postTitle(p)}</span>
              <code className="text-xs text-muted-foreground">{p.slug}</code>
            </Link>
          )
        },
      },
      {
        accessorKey: 'status',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.status),
        cell: ({ row }) => statusBadge(row.original.status),
      },
      {
        id: 'author',
        accessorFn: (row) => row.author?.id ?? '',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Author" />,
        filterFn: (row, _id, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.author?.id ?? '')
        },
        cell: ({ row }) =>
          row.original.author ? (
            <span className="text-sm">{row.original.author.displayName}</span>
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        id: 'stores',
        accessorFn: (row) => row.channelListings.filter((l) => l.isPublished).map((l) => l.channelId).join(','),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Published on" />,
        filterFn: (row, _id, value: string[]) => {
          if (!value?.length) return true
          const ids = new Set(row.original.channelListings.filter((l) => l.isPublished).map((l) => l.channelId))
          return value.some((v) => ids.has(v))
        },
        cell: ({ row }) => {
          const enabled = row.original.channelListings.filter((l) => l.isPublished)
          if (enabled.length === 0)
            return <span className="text-xs text-muted-foreground">- no stores -</span>
          return (
            <div className="flex flex-wrap gap-1">
              {enabled.map((l) => {
                const sv = stores.find((s) => s.id === l.channelId)
                return (
                  <Badge key={l.channelId} variant="secondary" className="font-normal">
                    {sv?.name ?? l.channelId.slice(0, 8)}
                  </Badge>
                )
              })}
            </div>
          )
        },
      },
      {
        id: 'categories',
        accessorFn: (row) =>
          row.categories.map((c) => c.content?.name ?? c.slug).join(', '),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Categories" />,
        cell: ({ row }) => {
          if (row.original.categories.length === 0)
            return <span className="text-xs text-muted-foreground">-</span>
          return (
            <div className="flex flex-wrap gap-1">
              {row.original.categories.slice(0, 3).map((c) => (
                <Badge key={c.id} variant="outline" className="font-normal">
                  {c.content?.name ?? c.slug}
                </Badge>
              ))}
              {row.original.categories.length > 3 && (
                <Badge variant="outline" className="font-normal text-muted-foreground">
                  +{row.original.categories.length - 3}
                </Badge>
              )}
            </div>
          )
        },
      },
      {
        accessorKey: 'publishedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Publish date" />,
        cell: ({ row }) =>
          row.original.publishedAt ? (
            new Date(row.original.publishedAt).toLocaleDateString()
          ) : (
            <span className="text-xs text-muted-foreground">-</span>
          ),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => new Date(row.original.updatedAt).toLocaleDateString(),
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
                aria-label={`Actions for ${postTitle(row.original)}`}
              >
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/blog/posts/${row.original.id}`}>Edit</Link>
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
                    Delete
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [stores, canManage],
  )

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    try {
      await deletePost({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <div className="grid gap-4">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-bold tracking-tight">Blog posts</h1>
        <p className="text-sm text-muted-foreground">
          Posts are global. Use the per-post Stores card to control where each one is published.
        </p>
      </div>

      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(r) => r.id}
        emptyState="No blog posts yet."
        searchPlaceholder="Search title or slug…"
        initialSorting={[{ id: 'updatedAt', desc: true }]}
        filters={[
          {
            column: 'status',
            title: 'Status',
            options: [
              { label: 'Draft', value: 'DRAFT' },
              { label: 'Scheduled', value: 'SCHEDULED' },
              { label: 'Published', value: 'PUBLISHED' },
              { label: 'Archived', value: 'ARCHIVED' },
            ],
          },
          {
            column: 'stores',
            title: 'Store',
            options: stores
              .filter((s) => s.isActive)
              .map((s) => ({ label: s.name, value: s.id })),
          },
          {
            column: 'author',
            title: 'Author',
            options: authors.map((a) => ({ label: a.displayName, value: a.id })),
          },
        ]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/blog/posts/new">
                <PlusIcon className="size-4" />
                New post
              </Link>
            </Button>
          )
        }
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
            <AlertDialogTitle>
              Delete {confirmingDelete ? postTitle(confirmingDelete) : 'post'}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the post and its translations. URL redirects are preserved.
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
              {deleting ? 'Deleting…' : 'Delete post'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
