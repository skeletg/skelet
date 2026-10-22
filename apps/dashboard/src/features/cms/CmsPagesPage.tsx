import { useCallback, useMemo, useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleOffIcon,
  CopyIcon,
  PlusIcon,
  TagIcon,
  Trash2Icon,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DataTable,
  DataTableColumnHeader,
  selectColumn,
  type BulkAction,
  useListUrlState,
} from '@/components/data-table'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  BulkDeleteCmsPagesDocument,
  BulkPublishCmsPagesDocument,
  BulkTagCmsPagesDocument,
  CloneCmsPageDocument,
  CmsPagesDocument,
  CmsTagsDocument,
  type CmsPageRowFragment,
} from './Cms.gql'
import { formatDate } from '@/lib/format'
import { workflowLabel } from './page-detail/shared'

function pageTitle(p: CmsPageRowFragment) {
  return p.content?.title?.trim() || p.allContent[0]?.title?.trim() || p.slug
}

type TagOps = Record<string, 'add' | 'remove'>

// BulkTagPicker is the per-tag add/remove control shown inside the "Edit tags" bulk-action confirm dialog - State is
// lifted to the page so the action's perform() can read the chosen ops
function BulkTagPicker({ value, onChange }: { value: TagOps; onChange: (v: TagOps) => void }) {
  const { data } = useQuery(CmsTagsDocument)
  const tags = data?.cmsTags ?? []
  if (tags.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No tags in the library yet - create some under CMS → Tags first.
      </p>
    )
  }
  function set(slug: string, op: 'add' | 'remove') {
    const next = { ...value }
    if (next[slug] === op) delete next[slug]
    else next[slug] = op
    onChange(next)
  }
  return (
    <div className="mt-2 max-h-64 space-y-1 overflow-y-auto">
      {tags.map((t) => (
        <div key={t.slug} className="flex items-center justify-between gap-2 rounded px-1 py-1">
          <span className="text-sm text-foreground">{t.name}</span>
          <div className="flex gap-1">
            <Button
              type="button"
              size="sm"
              variant={value[t.slug] === 'add' ? 'default' : 'outline'}
              className="h-7 px-2"
              onClick={() => set(t.slug, 'add')}
            >
              Add
            </Button>
            <Button
              type="button"
              size="sm"
              variant={value[t.slug] === 'remove' ? 'destructive' : 'outline'}
              className="h-7 px-2"
              onClick={() => set(t.slug, 'remove')}
            >
              Remove
            </Button>
          </div>
        </div>
      ))}
    </div>
  )
}

// CmsPagesPage lists CMS pages in a catalog-style advanced table with server-side full-text search (title + meta + block body), bulk
// actions (publish / unpublish / tag / delete), and a per-row Duplicate action - A row links to the editor
export function CmsPagesPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('cms.manage')
  const navigate = useNavigate()
  const { scope, selectedChannel } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || undefined
      : undefined
  const effectiveChannelId =
    scope.mode === 'CHANNEL' && scope.channelId ? scope.channelId : undefined

  // Debounce the search box so each keystroke doesn't refetch
  const [searchInput, setSearchInput] = useState('')
  const search = useDebounced(searchInput, 300).trim()

  const variables = useMemo(
    () => ({
      filter: {
        onlyPublished: false,
        ...(effectiveChannelId ? { channelId: effectiveChannelId } : {}),
        ...(search ? { search } : {}),
      },
      locale: scopedLocale ?? null,
      limit: 200,
      offset: 0,
    }),
    [scopedLocale, effectiveChannelId, search],
  )

  const { data, loading, error, refetch } = useQuery(CmsPagesDocument, { variables })
  const pages = data?.cmsPages ?? []

  const [bulkPublish] = useMutation(BulkPublishCmsPagesDocument)
  const [bulkTag] = useMutation(BulkTagCmsPagesDocument)
  const [bulkDelete] = useMutation(BulkDeleteCmsPagesDocument)
  const [clonePage] = useMutation(CloneCmsPageDocument)

  const [tagOps, setTagOps] = useState<TagOps>({})
  const [cloningId, setCloningId] = useState<string | null>(null)

  const onDuplicate = useCallback(
    async (id: string) => {
      setCloningId(id)
      try {
        const res = await clonePage({ variables: { id } })
        const newId = res.data?.cloneCMSPage?.id
        if (newId) navigate(`/cms/pages/${newId}`)
      } finally {
        setCloningId(null)
      }
    },
    [clonePage, navigate],
  )

  const bulkActions = useMemo<BulkAction<CmsPageRowFragment>[] | undefined>(() => {
    if (!canManage) return undefined
    const ids = (rows: CmsPageRowFragment[]) => rows.map((r) => r.id)
    return [
      {
        id: 'publish',
        label: 'Publish',
        icon: CheckCircle2Icon,
        perform: async (rows) => {
          await bulkPublish({ variables: { ids: ids(rows), published: true } })
          await refetch()
        },
      },
      {
        id: 'unpublish',
        label: 'Unpublish',
        icon: CircleOffIcon,
        variant: 'secondary',
        perform: async (rows) => {
          await bulkPublish({ variables: { ids: ids(rows), published: false } })
          await refetch()
        },
      },
      {
        id: 'tags',
        label: 'Edit tags',
        icon: TagIcon,
        variant: 'secondary',
        confirm: {
          title: (rows) => `Edit tags on ${rows.length} page(s)`,
          description: () => <BulkTagPicker value={tagOps} onChange={setTagOps} />,
          actionLabel: 'Apply',
        },
        perform: async (rows) => {
          const addTags = Object.entries(tagOps).filter(([, v]) => v === 'add').map(([s]) => s)
          const removeTags = Object.entries(tagOps).filter(([, v]) => v === 'remove').map(([s]) => s)
          await bulkTag({ variables: { ids: ids(rows), addTags, removeTags } })
          setTagOps({})
          await refetch()
        },
      },
      {
        id: 'delete',
        label: 'Delete',
        icon: Trash2Icon,
        variant: 'destructive',
        confirm: {
          title: (rows) => `Delete ${rows.length} page(s)?`,
          description:
            "This can't be undone. A page that is the parent of another page can't be deleted - that cancels the whole batch.",
          actionLabel: 'Delete',
        },
        perform: async (rows) => {
          await bulkDelete({ variables: { ids: ids(rows) } })
          await refetch()
        },
      },
    ]
  }, [canManage, bulkPublish, bulkTag, bulkDelete, refetch, tagOps])

  const columns = useMemo<ColumnDef<CmsPageRowFragment>[]>(() => {
    const cols: ColumnDef<CmsPageRowFragment>[] = []
    if (canManage) cols.push(selectColumn<CmsPageRowFragment>())
    cols.push(
      {
        id: 'title',
        accessorFn: (p) => pageTitle(p),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
        cell: ({ row }) => (
          <Link to={`/cms/pages/${row.original.id}`} className="font-medium hover:underline">
            {pageTitle(row.original)}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        accessorKey: 'urlPath',
        header: ({ column }) => <DataTableColumnHeader column={column} title="URL path" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.urlPath}</span>,
      },
      {
        id: 'status',
        accessorFn: (p) => (p.isPublished ? 'published' : 'draft'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.isPublished ? 'published' : 'draft'),
        cell: ({ row }) =>
          row.original.isPublished ? (
            <span className="inline-flex items-center gap-1.5 text-sm text-success">
              <CheckCircle2Icon className="size-3.5" /> Published
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
              <CircleOffIcon className="size-3.5" /> Draft
            </span>
          ),
      },
      {
        id: 'workflow',
        accessorFn: (p) => p.workflowState,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Workflow" />,
        filterFn: (row, _id, value: string[]) => !value?.length || value.includes(row.original.workflowState),
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{workflowLabel(row.original.workflowState)}</span>
        ),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-sm text-muted-foreground">{formatDate(row.original.updatedAt)}</span>
        ),
      },
    )
    if (canManage) {
      cols.push({
        id: 'actions',
        header: () => null,
        enableSorting: false,
        cell: ({ row }) => (
          <Button
            size="sm"
            variant="ghost"
            className="h-7 px-2 text-muted-foreground"
            disabled={cloningId === row.original.id}
            onClick={() => onDuplicate(row.original.id)}
          >
            <CopyIcon className="size-4" />
            {cloningId === row.original.id ? 'Duplicating…' : 'Duplicate'}
          </Button>
        ),
      })
    }
    return cols
  }, [canManage, cloningId, onDuplicate])

  return (
    <DataTable
      urlState={urlState}
      columns={columns}
      data={pages}
      loading={loading}
      error={error ? { message: error.message } : null}
      getRowId={(p) => p.id}
      enableRowSelection={canManage}
      bulkActions={bulkActions}
      emptyState={search ? `No pages match “${search}”.` : 'No pages yet.'}
      initialSorting={[{ id: 'updatedAt', desc: true }]}
      toolbarSlot={() => (
        <Input
          placeholder="Search title, meta, or page content…"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="h-8 w-full sm:w-72"
        />
      )}
      filters={[
        {
          column: 'status',
          title: 'Status',
          options: [
            { label: 'Published', value: 'published' },
            { label: 'Draft', value: 'draft' },
          ],
        },
        {
          column: 'workflow',
          title: 'Workflow',
          options: [
            { label: 'Draft', value: 'DRAFT' },
            { label: 'In review', value: 'IN_REVIEW' },
            { label: 'Approved', value: 'APPROVED' },
            { label: 'Published', value: 'PUBLISHED' },
          ],
        },
      ]}
      toolbarRightSlot={
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" asChild>
            <Link to="/cms/tags">
              <TagIcon className="size-4" />
              Manage tags
            </Link>
          </Button>
          {canManage && (
            <Button size="sm" asChild>
              <Link to="/cms/pages/new">
                <PlusIcon className="size-4" />
                New page
              </Link>
            </Button>
          )}
        </div>
      }
    />
  )
}
