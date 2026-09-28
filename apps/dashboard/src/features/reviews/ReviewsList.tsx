import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { LoadError } from '@/shared/errors/LoadError'
import type { ReviewStatus } from '@/graphql/schema.types'
import {
  ReviewCategoriesDocument,
  ReviewModerationTagsDocument,
  ReviewsDocument,
  type ReviewFieldsFragment,
} from './Reviews.gql'
import { ProductFilter } from './ProductFilter'

const PAGE_SIZE = 50
// Facet value standing in for reviews with no assigned category
const UNCATEGORIZED_VALUE = '__none__'
// Mod facet sentinels for reviews that are checked-and-clean / not yet checked
const CLEAN_VALUE = '__clean__'
const UNCHECKED_VALUE = '__unchecked__'

const STATUS_TABS: { label: string; value: ReviewStatus | '' }[] = [
  { label: 'All', value: '' },
  { label: 'Pending', value: 'PENDING' },
  { label: 'Approved', value: 'APPROVED' },
  { label: 'Rejected', value: 'REJECTED' },
]

// Moderation queue - The Status filter is applied SERVER-SIDE (channel-scoped),
// so it reaches every review of that status - not just a capped first page -
// and "Load more" widens the window via the query's limit - The Verified and
// AI-verdict facets stay client-side over the loaded window (they're display
// refinements, not the axis that scales). `defaultStatus` seeds the initial
// Status tab - No inline header - DashboardLayout's top bar covers it

export function ReviewsList({
  defaultStatus,
}: {
  defaultStatus?: ReviewStatus[]
}) {
  const [status, setStatus] = useState<ReviewStatus | ''>(defaultStatus?.[0] ?? '')
  const [product, setProduct] = useState<{ id: string; name: string } | null>(null)
  const [limit, setLimit] = useState(PAGE_SIZE)

  // status + product are filtered server-side (channel-scoped); both reach
  // every matching review, not just the loaded window
  const filter = useMemo(() => {
    const f: { status?: ReviewStatus[]; productId?: string } = {}
    if (status) f.status = [status]
    if (product) f.productId = product.id
    return Object.keys(f).length ? f : null
  }, [status, product])

  const { data, loading, error, refetch } = useQuery(ReviewsDocument, {
    variables: { filter, limit, offset: 0 },
    notifyOnNetworkStatusChange: true,
  })

  // Categories + moderation tags are small cached lists - used to label the
  // category/Mod columns and populate their facets (reviews carry only ids/slugs)
  const { data: catData } = useQuery(ReviewCategoriesDocument, { fetchPolicy: 'cache-first' })
  const categories = useMemo(() => catData?.reviewCategories ?? [], [catData])
  const categoryName = useMemo(() => {
    const m = new Map<string, string>()
    for (const c of categories) m.set(c.slug, c.name)
    return m
  }, [categories])

  const { data: tagData } = useQuery(ReviewModerationTagsDocument, { fetchPolicy: 'cache-first' })
  const modTags = useMemo(() => tagData?.reviewModerationTags ?? [], [tagData])
  const tagName = useMemo(() => {
    const m = new Map<string, string>()
    for (const t of modTags) m.set(t.slug, t.name)
    return m
  }, [modTags])

  const rows = (data?.reviews.items ?? []) as ReviewFieldsFragment[]
  const total = data?.reviews.totalCount ?? 0
  const canLoadMore = rows.length < total

  const selectStatus = (next: ReviewStatus | '') => {
    setStatus(next)
    setLimit(PAGE_SIZE) // reset the window when the server-side filter changes
  }
  const selectProduct = (p: { id: string; name: string } | null) => {
    setProduct(p)
    setLimit(PAGE_SIZE)
  }

  const statusTabs = (
    <div className="flex flex-wrap items-center gap-2">
      <ProductFilter product={product} onChange={selectProduct} />
      <div className="flex flex-wrap gap-1">
        {STATUS_TABS.map((tab) => (
          <Button
            key={tab.value || 'all'}
            variant={status === tab.value ? 'default' : 'outline'}
            size="sm"
            onClick={() => selectStatus(tab.value)}
          >
            {tab.label}
          </Button>
        ))}
      </div>
    </div>
  )

  const columns = useMemo<ColumnDef<ReviewFieldsFragment>[]>(
    () => [
      {
        accessorKey: 'submittedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Submitted" />,
        cell: ({ row }) => (
          <Link
            to={`/reviews/${row.original.id}`}
            className="font-medium hover:underline"
          >
            {new Date(row.original.submittedAt).toLocaleString()}
          </Link>
        ),
      },
      {
        // Status is filtered server-side (see statusTabs), so this column is
        // display-only - no client filterFn / facet
        id: 'status',
        accessorFn: (row) => row.status,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        cell: ({ row }) => <ReviewStatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'title',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Title" />,
        cell: ({ row }) => (
          <span className="text-sm">
            {row.original.title || <em className="text-muted-foreground">(no title)</em>}
          </span>
        ),
      },
      {
        id: 'verified',
        accessorFn: (row) => (row.isVerifiedPurchase ? 'verified' : 'unverified'),
        header: 'Verified',
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.isVerifiedPurchase ? 'verified' : 'unverified')
        },
        cell: ({ row }) =>
          row.original.isVerifiedPurchase ? (
            <Badge variant="secondary">Purchased</Badge>
          ) : (
            <span className="text-xs text-muted-foreground">No</span>
          ),
      },
      {
        // Mod = the AI's moderation flags (multi-tag) - Facet values are tag
        // slugs plus sentinels for checked-clean / not-yet-checked
        id: 'mod',
        accessorFn: (row) => row.moderationTags.join(',') || row.moderationLabel || UNCHECKED_VALUE,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Mod" />,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          const tags = row.original.moderationTags
          for (const v of value) {
            if (v === CLEAN_VALUE && tags.length === 0 && row.original.moderationLabel === 'genuine') return true
            if (v === UNCHECKED_VALUE && row.original.moderationLabel === '') return true
            if (tags.includes(v)) return true
          }
          return false
        },
        cell: ({ row }) => <ReviewModTags tags={row.original.moderationTags} label={row.original.moderationLabel} nameOf={tagName} />,
      },
      {
        // Multi-category: a review can be in several - Facet matches any selected
        // slug; the Uncategorized sentinel matches reviews with no category
        id: 'category',
        accessorFn: (row) => row.categorySlugs.join(',') || UNCATEGORIZED_VALUE,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Category" />,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          const slugs = row.original.categorySlugs
          for (const v of value) {
            if (v === UNCATEGORIZED_VALUE && slugs.length === 0) return true
            if (slugs.includes(v)) return true
          }
          return false
        },
        cell: ({ row }) => {
          const slugs = row.original.categorySlugs
          if (slugs.length === 0) return <span className="text-xs text-muted-foreground">-</span>
          return (
            <div className="flex flex-wrap gap-1">
              {slugs.map((slug) => (
                <Badge key={slug} variant="outline">
                  {categoryName.get(slug) ?? slug}
                </Badge>
              ))}
            </div>
          )
        },
      },
      {
        accessorKey: 'media',
        header: 'Media',
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.media.length}
          </span>
        ),
      },
      {
        accessorKey: 'productId',
        header: 'Product',
        cell: ({ row }) => (
          <Link
            to={`/products/${row.original.productId}`}
            className="font-mono text-[10px] text-muted-foreground hover:underline"
          >
            {row.original.productId.slice(0, 8)}…
          </Link>
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
                aria-label="Review actions"
              >
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/reviews/${row.original.id}`}>View</Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link to={`/products/${row.original.productId}`}>Open product</Link>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [categoryName, tagName],
  )

  if (error) {
    return (
      <LoadError
        title="Couldn't load reviews"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  }

  return (
    <div className="grid min-w-0 gap-3">
      <DataTable
        columns={columns}
        data={rows}
        loading={loading}
        getRowId={(row) => row.id}
        emptyState="No reviews."
        searchPlaceholder="Search title…"
        initialSorting={[{ id: 'submittedAt', desc: true }]}
        toolbarRightSlot={statusTabs}
        filters={[
          {
            column: 'verified',
            title: 'Verified',
            options: [
              { label: 'Verified purchase', value: 'verified' },
              { label: 'Unverified', value: 'unverified' },
            ],
          },
          {
            column: 'mod',
            title: 'Mod',
            options: [
              ...modTags.map((t) => ({ label: t.name, value: t.slug })),
              { label: 'Clean', value: CLEAN_VALUE },
              { label: 'Not checked', value: UNCHECKED_VALUE },
            ],
          },
          {
            column: 'category',
            title: 'Category',
            options: [
              ...categories.map((c) => ({ label: c.name, value: c.slug })),
              { label: 'Uncategorized', value: UNCATEGORIZED_VALUE },
            ],
          },
        ]}
      />

      <div className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
        <span>
          Showing {rows.length} of {total}
        </span>
        {canLoadMore ? (
          <Button
            variant="outline"
            size="sm"
            disabled={loading}
            onClick={() => setLimit((l) => l + PAGE_SIZE)}
          >
            {loading ? 'Loading…' : 'Load more'}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

export function ReviewStatusBadge({ status }: { status: ReviewStatus }) {
  switch (status) {
    case 'APPROVED':
      return <Badge className="bg-success text-success-foreground hover:bg-success/90">Approved</Badge>
    case 'REJECTED':
      return <Badge variant="destructive">Rejected</Badge>
    default:
      return <Badge variant="secondary">Pending</Badge>
  }
}

// ReviewModerationBadge renders the coarse AI verdict (suggestion only) - The current model derives "genuine" (no
// concerns) / "flagged" (has tags); older rows may still carry spam/abusive/off_topic - Empty label renders nothing
export function ReviewModerationBadge({ label }: { label: string }) {
  switch (label) {
    case 'genuine':
      return <Badge className="bg-success text-success-foreground hover:bg-success/90">Genuine</Badge>
    case 'flagged':
      return <Badge className="bg-warning text-warning-foreground hover:bg-warning/90">Flagged</Badge>
    case 'spam':
      return <Badge variant="destructive">Spam</Badge>
    case 'abusive':
      return <Badge variant="destructive">Abusive</Badge>
    case 'off_topic':
      return <Badge className="bg-warning text-warning-foreground hover:bg-warning/90">Off-topic</Badge>
    case '':
      return null
    default:
      return <Badge variant="outline">{label}</Badge>
  }
}

// ReviewModTags renders the per-review moderation flag badges (resolving slugs to tag names) - No tags + a "genuine"
// verdict reads as a muted "Clean"; an unchecked review (empty label) renders a dash
export function ReviewModTags({
  tags,
  label,
  nameOf,
}: {
  tags: string[]
  label: string
  nameOf: Map<string, string>
}) {
  if (tags.length > 0) {
    return (
      <div className="flex flex-wrap gap-1">
        {tags.map((slug) => (
          <Badge key={slug} variant="destructive" className="text-[10px]">
            {nameOf.get(slug) ?? slug}
          </Badge>
        ))}
      </div>
    )
  }
  if (label === 'genuine') return <Badge variant="secondary">Clean</Badge>
  return <span className="text-xs text-muted-foreground">-</span>
}
