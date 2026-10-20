import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { MoreHorizontalIcon, PlusIcon, ShieldAlertIcon, Trash2Icon } from 'lucide-react'
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
  DeleteReviewModerationTagDocument,
  ReviewModerationTagsDocument,
  type ReviewModerationTagFieldsFragment,
} from './Reviews.gql'
import { ReviewModerationTagFormDialog } from './ReviewModerationTagFormDialog'

// Operator-managed abstract moderation-concern taxonomy the AI flags on reviews (profanity, spam, off-topic, …) - A
// review can carry several - Same flat CRUD shape as categories; rendered as a tab inside the Reviews shell
export function ReviewModerationTagsContent() {
  const { has } = usePermissions()
  const canManage = has('reviews.manage')

  const { data, loading, error, refetch } = useQuery(ReviewModerationTagsDocument, {
    notifyOnNetworkStatusChange: true,
  })
  const rows = useMemo(
    () => (data?.reviewModerationTags ?? []) as ReviewModerationTagFieldsFragment[],
    [data],
  )
  const ordered = useMemo(
    () => [...rows].sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name)),
    [rows],
  )

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ReviewModerationTagFieldsFragment | null>(null)
  const [confirmingDelete, setConfirmingDelete] =
    useState<ReviewModerationTagFieldsFragment | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteTag, { loading: deleting }] = useMutation(DeleteReviewModerationTagDocument, {
    refetchQueries: [ReviewModerationTagsDocument],
  })

  function openCreate() {
    setEditing(null)
    setFormOpen(true)
  }
  function openEdit(t: ReviewModerationTagFieldsFragment) {
    setEditing(t)
    setFormOpen(true)
  }

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteTag({ variables: { id: confirmingDelete.id } })
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
            title="Couldn't load moderation tags"
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
        title={`Moderation tags (${rows.length})`}
        description="Concerns the AI flags on reviews during moderation (profanity, spam, off-topic, …). A review can carry several. Edit the descriptions to steer the classifier."
        trailing={
          canManage ? (
            <Button size="sm" onClick={openCreate}>
              <PlusIcon className="size-4" />
              New tag
            </Button>
          ) : null
        }
      >
        {loading && rows.length === 0 ? (
          <p className="py-6 text-sm text-muted-foreground">Loading…</p>
        ) : rows.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 rounded border border-dashed py-12 text-center">
            <ShieldAlertIcon className="size-6 text-muted-foreground/60" />
            <p className="text-sm font-medium">No moderation tags yet</p>
            <p className="max-w-sm text-xs text-muted-foreground">
              Create concerns (e.g. profanity, spam, off-topic) and the AI will flag matching reviews
              during moderation.
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
            {ordered.map((t) => (
              <li key={t.id} className="flex items-start gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium">{t.name}</span>
                    <span className="font-mono text-[10px] text-muted-foreground">{t.slug}</span>
                    {!t.isActive && (
                      <Badge variant="outline" className="text-muted-foreground">
                        Inactive
                      </Badge>
                    )}
                  </div>
                  {t.description ? (
                    <p className="mt-1 text-xs text-muted-foreground">{t.description}</p>
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
                        aria-label={`Actions for ${t.name}`}
                      >
                        <MoreHorizontalIcon className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => openEdit(t)}>Edit</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onSelect={() => {
                          setDeleteError(null)
                          setConfirmingDelete(t)
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

      <ReviewModerationTagFormDialog open={formOpen} onOpenChange={setFormOpen} tag={editing} />

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
              Reviews already flagged with this tag keep the raw slug (shown as-is) until re-checked.
              The AI won't apply it to new reviews. This is permanent.
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
              {deleting ? 'Deleting…' : 'Delete tag'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageBody>
  )
}
