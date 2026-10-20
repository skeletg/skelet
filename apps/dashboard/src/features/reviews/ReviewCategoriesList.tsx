import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { MoreHorizontalIcon, PlusIcon, TagsIcon, Trash2Icon } from 'lucide-react'
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
import { DetailPageBody, DetailPageSection } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeleteReviewCategoryDocument,
  ReviewCategoriesDocument,
  type ReviewCategoryFieldsFragment,
} from './Reviews.gql'
import { ReviewCategoryFormDialog } from './ReviewCategoryFormDialog'

// Operator-managed taxonomy the AI classifier sorts reviews into - Flat list
// (no per-locale content or product-type assignment, unlike review questions),
// so a single-pane list with create/edit/delete dialogs is enough - no
// master/detail split - Sorted by sortOrder then name to mirror how the backend
// hands them to the classifier - Rendered as a tab inside the Reviews shell
export function ReviewCategoriesContent() {
  const { has } = usePermissions()
  const canManage = has('reviews.manage')

  const { data, loading, error, refetch } = useQuery(ReviewCategoriesDocument, {
    notifyOnNetworkStatusChange: true,
  })
  const rows = useMemo(
    () => (data?.reviewCategories ?? []) as ReviewCategoryFieldsFragment[],
    [data],
  )
  const ordered = useMemo(
    () =>
      [...rows].sort(
        (a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name),
      ),
    [rows],
  )

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ReviewCategoryFieldsFragment | null>(null)
  const [confirmingDelete, setConfirmingDelete] =
    useState<ReviewCategoryFieldsFragment | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteCategory, { loading: deleting }] = useMutation(DeleteReviewCategoryDocument, {
    refetchQueries: [ReviewCategoriesDocument],
  })

  function openCreate() {
    setEditing(null)
    setFormOpen(true)
  }
  function openEdit(c: ReviewCategoryFieldsFragment) {
    setEditing(c)
    setFormOpen(true)
  }

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteCategory({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (error) {
    return (
      <DetailPageBody>
        <DetailPageSection>
          <LoadError
            title="Couldn't load review categories"
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
        title={`Categories (${rows.length})`}
        description="Topical groups the AI sorts each review into during moderation. Edit the descriptions to steer the classifier; add or retire groups as your feedback evolves."
        trailing={
          canManage ? (
            <Button size="sm" onClick={openCreate}>
              <PlusIcon className="size-4" />
              New category
            </Button>
          ) : null
        }
      >
        {loading && rows.length === 0 ? (
            <p className="py-6 text-sm text-muted-foreground">Loading…</p>
          ) : rows.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-2 rounded border border-dashed py-12 text-center">
              <TagsIcon className="size-6 text-muted-foreground/60" />
              <p className="text-sm font-medium">No categories yet</p>
              <p className="max-w-sm text-xs text-muted-foreground">
                Create a few groups (e.g. late shipment, product broken, happy customer) and the AI
                will start sorting reviews into them.
              </p>
              {canManage && (
                <Button size="sm" variant="outline" onClick={openCreate}>
                  <PlusIcon className="size-3.5" />
                  Create your first
                </Button>
              )}
            </div>
          ) : (
            <ul className="divide-y rounded border">
              {ordered.map((c) => (
                <li key={c.id} className="flex items-start gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{c.name}</span>
                      <span className="font-mono text-[10px] text-muted-foreground">{c.slug}</span>
                      {!c.isActive && (
                        <Badge variant="outline" className="text-muted-foreground">
                          Inactive
                        </Badge>
                      )}
                    </div>
                    {c.description ? (
                      <p className="mt-1 text-xs text-muted-foreground">{c.description}</p>
                    ) : (
                      <p className="mt-1 text-xs italic text-muted-foreground/70">
                        No description - the AI has nothing to match against.
                      </p>
                    )}
                  </div>
                  {canManage && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="size-8 shrink-0 text-muted-foreground"
                          aria-label={`Actions for ${c.name}`}
                        >
                          <MoreHorizontalIcon className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onSelect={() => openEdit(c)}>Edit</DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onSelect={() => {
                            setDeleteError(null)
                            setConfirmingDelete(c)
                          }}
                        >
                          <Trash2Icon className="size-4" />
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </li>
              ))}
            </ul>
          )}
        </DetailPageSection>

      <ReviewCategoryFormDialog open={formOpen} onOpenChange={setFormOpen} category={editing} />

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
            <AlertDialogTitle>Delete &quot;{confirmingDelete?.name}&quot;?</AlertDialogTitle>
            <AlertDialogDescription>
              Reviews currently in this group become uncategorized (their category is cleared). The
              AI won't reassign them automatically - re-check a review or run a backfill to
              reclassify. This is permanent.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Couldn't delete</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                void onConfirmDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete category'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageBody>
  )
}
