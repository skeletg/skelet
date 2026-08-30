import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { CheckCircle2Icon, CircleOffIcon, PlusIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { useConfigScope } from '@/lib/config-scope'
import { CmsPagesDocument, type CmsPageRowFragment } from './Cms.gql'

function pageTitle(p: CmsPageRowFragment) {
  return p.content?.title?.trim() || p.allContent[0]?.title?.trim() || p.slug
}

function fmtWhen(v: unknown): string {
  const d = new Date(v as string)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString(undefined, { dateStyle: 'medium' })
}

// PageTypePagesPanel lists every page of one page type in an advanced table
// (search / sort / status filter / tag filter), mirroring the global pages
// list - Rows link to the page editor; the "New page" action deep-links the
// create form with this type prefilled - In CHANNEL scope it narrows to pages
// listed on the selected channel, matching the main pages list
export function PageTypePagesPanel({
  pageTypeId,
  canManage,
}: {
  pageTypeId: string
  canManage: boolean
}) {
  const { scope } = useConfigScope()
  const effectiveChannelId =
    scope.mode === 'CHANNEL' && scope.channelId ? scope.channelId : undefined

  const variables = useMemo(
    () => ({
      filter: {
        pageTypeId,
        onlyPublished: false,
        ...(effectiveChannelId ? { channelId: effectiveChannelId } : {}),
      },
      limit: 200,
      offset: 0,
    }),
    [pageTypeId, effectiveChannelId],
  )

  const { data, loading, error } = useQuery(CmsPagesDocument, { variables, skip: !pageTypeId })
  const pages = useMemo(() => data?.cmsPages ?? [], [data])

  // Distinct tags across this type's pages → faceted filter options
  const tagOptions = useMemo(() => {
    const set = new Set<string>()
    for (const p of pages) for (const t of p.tags) set.add(t)
    return Array.from(set)
      .sort()
      .map((t) => ({ label: t, value: t }))
  }, [pages])

  const columns = useMemo<ColumnDef<CmsPageRowFragment>[]>(
    () => [
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
        accessorKey: 'urlPath',
        header: ({ column }) => <DataTableColumnHeader column={column} title="URL path" />,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.urlPath}</span>
        ),
      },
      {
        id: 'tags',
        accessorFn: (p) => p.tags,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Tags" />,
        enableSorting: false,
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.some((v) => row.original.tags.includes(v)),
        cell: ({ row }) =>
          row.original.tags.length === 0 ? (
            <span className="text-muted-foreground">-</span>
          ) : (
            <div className="flex flex-wrap gap-1">
              {row.original.tags.map((t) => (
                <Badge key={t} variant="secondary" className="font-normal">
                  {t}
                </Badge>
              ))}
            </div>
          ),
      },
      {
        id: 'status',
        accessorFn: (p) => (p.isPublished ? 'published' : 'draft'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.isPublished ? 'published' : 'draft'),
        cell: ({ row }) =>
          row.original.isPublished ? (
            <span className="inline-flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-400">
              <CheckCircle2Icon className="size-3.5" /> Published
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
              <CircleOffIcon className="size-3.5" /> Draft
            </span>
          ),
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-sm text-muted-foreground">
            {fmtWhen(row.original.updatedAt)}
          </span>
        ),
      },
    ],
    [],
  )

  return (
    <DataTable
      columns={columns}
      data={pages}
      loading={loading}
      error={error ? { message: error.message } : null}
      getRowId={(p) => p.id}
      emptyState="No pages of this type yet."
      searchPlaceholder="Search title or URL…"
      pageSize={10}
      initialSorting={[{ id: 'updatedAt', desc: true }]}
      filters={[
        {
          column: 'status',
          title: 'Status',
          options: [
            { label: 'Published', value: 'published' },
            { label: 'Draft', value: 'draft' },
          ],
        },
        ...(tagOptions.length > 0
          ? [{ column: 'tags', title: 'Tag', options: tagOptions }]
          : []),
      ]}
      toolbarRightSlot={
        canManage && (
          <Button size="sm" asChild>
            <Link to={`/cms/pages/new?pageTypeId=${pageTypeId}`}>
              <PlusIcon className="size-4" />
              New page
            </Link>
          </Button>
        )
      }
    />
  )
}
