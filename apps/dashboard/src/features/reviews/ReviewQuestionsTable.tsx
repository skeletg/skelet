import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MoreHorizontalIcon, PlusIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { DetailPageBody, DetailPageSection } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ProductFilter } from './ProductFilter'
import {
  DeleteReviewQuestionDocument,
  ReviewQuestionsDocument,
  ReviewQuestionsForProductDocument,
  type ReviewQuestionFieldsFragment,
} from './Reviews.gql'
import { ReviewQuestionFormDialog } from './ReviewQuestionFormDialog'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

const TYPE_LABELS: Record<string, string> = {
  RATING_1_5: 'Rating 1–5',
  YES_NO: 'Yes / No',
  FREE_TEXT: 'Free text',
}

// ReviewQuestionsTable is the Questions tab: the question library as a table, with an optional per-product filter
// (shows the questions assigned to that product's type) - Create is a dialog; a row opens the per-locale edit page
export function ReviewQuestionsTable() {
  const { has } = usePermissions()
  const canManage = has('reviews.manage')

  const [product, setProduct] = useState<{ id: string; name: string } | null>(null)

  // All questions, or just those applicable to the selected product
  const allQ = useQuery(ReviewQuestionsDocument, { notifyOnNetworkStatusChange: true })
  const prodQ = useQuery(ReviewQuestionsForProductDocument, {
    variables: { productId: product?.id ?? '' },
    skip: !product,
    notifyOnNetworkStatusChange: true,
  })

  const loading = product ? prodQ.loading : allQ.loading
  const error = product ? prodQ.error : allQ.error
  const refetch = product ? prodQ.refetch : allQ.refetch
  const rows = useMemo(
    () =>
      (product
        ? (prodQ.data?.reviewQuestionsForProduct ?? [])
        : (allQ.data?.reviewQuestions ?? [])) as ReviewQuestionFieldsFragment[],
    [product, prodQ.data, allQ.data],
  )

  const [createOpen, setCreateOpen] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState<ReviewQuestionFieldsFragment | null>(null)
  const [deleteQuestion] = useMutation(DeleteReviewQuestionDocument, {
    refetchQueries: [ReviewQuestionsDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteQuestion({ variables: { id: confirmingDelete.id } })
  }

  const columns = useMemo<ColumnDef<ReviewQuestionFieldsFragment>[]>(
    () => [
      {
        accessorKey: 'label',
        accessorFn: (row) => row.content?.label || row.slug,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Question" />,
        cell: ({ row }) => (
          <Link to={`/reviews/questions/${row.original.id}`} className="font-medium hover:underline">
            {row.original.content?.label || row.original.slug}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: 'Slug',
        cell: ({ row }) => <span className="font-mono text-[11px] text-muted-foreground">{row.original.slug}</span>,
      },
      {
        id: 'type',
        accessorFn: (row) => row.inputType,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Type" />,
        filterFn: (row, _id, value: string[]) => !value?.length || value.includes(row.original.inputType),
        cell: ({ row }) => <Badge variant="secondary">{TYPE_LABELS[row.original.inputType] ?? row.original.inputType}</Badge>,
      },
      {
        id: 'required',
        accessorFn: (row) => (row.isRequired ? 'required' : 'optional'),
        header: 'Required',
        filterFn: (row, _id, value: string[]) => !value?.length || value.includes(row.original.isRequired ? 'required' : 'optional'),
        cell: ({ row }) =>
          row.original.isRequired ? <Badge>Required</Badge> : <span className="text-xs text-muted-foreground">Optional</span>,
      },
      {
        accessorKey: 'allowComment',
        header: 'Comment',
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{row.original.allowComment ? 'Allowed' : '-'}</span>
        ),
      },
      {
        id: 'locales',
        accessorFn: (row) => row.allContent.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Locales" />,
        cell: ({ row }) => <span className="tabular-nums text-muted-foreground">{row.original.allContent.length}</span>,
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="size-8" aria-label="Question actions">
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/reviews/questions/${row.original.id}`}>Open</Link>
              </DropdownMenuItem>
              {canManage ? (
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => {
                    setConfirmingDelete(row.original)
                  }}
                >
                  Delete
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [canManage],
  )

  const toolbar = (
    <div className="flex flex-wrap items-center gap-2">
      <ProductFilter product={product} onChange={setProduct} />
      {canManage ? (
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <PlusIcon className="size-4" />
          New question
        </Button>
      ) : null}
    </div>
  )

  if (error) {
    return (
      <DetailPageBody>
        <DetailPageSection>
          <LoadError
            title="Couldn't load review questions"
            message={error.message}
            onRetry={() => void refetch()}
            retrying={loading}
          />
        </DetailPageSection>
      </DetailPageBody>
    )
  }

  return (
    <DetailPageBody>
      <DetailPageSection
        title="Review questions"
        description={
          product
            ? `Questions shown on “${product.name}” (assigned to its product type).`
            : 'The question library - the per-product prompts shown on storefront review forms. Filter by product to see what applies where.'
        }
      >
        <DataTable
          columns={columns}
          data={rows}
          loading={loading}
          getRowId={(row) => row.id}
          emptyState={product ? 'No questions assigned to this product’s type.' : 'No questions yet.'}
          searchPlaceholder="Search label or slug…"
          initialSorting={[{ id: 'label', desc: false }]}
          toolbarRightSlot={toolbar}
          filters={[
            {
              column: 'type',
              title: 'Type',
              options: [
                { label: 'Rating 1–5', value: 'RATING_1_5' },
                { label: 'Yes / No', value: 'YES_NO' },
                { label: 'Free text', value: 'FREE_TEXT' },
              ],
            },
            {
              column: 'required',
              title: 'Required',
              options: [
                { label: 'Required', value: 'required' },
                { label: 'Optional', value: 'optional' },
              ],
            },
          ]}
        />
      </DetailPageSection>

      <ReviewQuestionFormDialog open={createOpen} onOpenChange={setCreateOpen} attribute={null} />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete &quot;{confirmingDelete?.content?.label || confirmingDelete?.slug}&quot;?</>}
        description="The question is unassigned from every product type and its answers on past reviews are deleted. This is permanent."
        confirmLabel="Delete question"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </DetailPageBody>
  )
}
