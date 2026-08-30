import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  CheckIcon,
  MessageCircleQuestionIcon,
  MoreHorizontalIcon,
  PlusIcon,
  SearchIcon,
  Trash2Icon,
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
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { DetailPageLayout } from '@/components/layout/detail-page'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeleteReviewQuestionDocument,
  ReviewQuestionsDocument,
  type ReviewQuestionFieldsFragment,
} from './Reviews.gql'
import { ReviewQuestionFormDialog } from './ReviewQuestionFormDialog'
import { ReviewQuestionDetail } from './ReviewQuestionDetail'

// ReviewQuestionsList is the master/detail shell for /reviews/questions and /reviews/questions/:id - Create is a dialog
// (the existing ReviewQuestionFormDialog handles both create + edit), so the sidebar's "New" button opens it inline
export function ReviewQuestionsList() {
  const navigate = useNavigate()
  const params = useParams<{ id?: string }>()
  const activeID = params.id ?? null

  const { has } = usePermissions()
  const canManage = has('reviews.manage')

  const { data, loading, error, refetch } = useQuery(ReviewQuestionsDocument, {
    notifyOnNetworkStatusChange: true,
  })
  const rows = useMemo(
    () => (data?.reviewQuestions ?? []) as ReviewQuestionFieldsFragment[],
    [data],
  )

  const [query, setQuery] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [confirmingDelete, setConfirmingDelete] =
    useState<ReviewQuestionFieldsFragment | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteQuestion, { loading: deleting }] = useMutation(
    DeleteReviewQuestionDocument,
    { refetchQueries: [{ query: ReviewQuestionsDocument }] },
  )

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteQuestion({ variables: { id: confirmingDelete.id } })
      if (activeID === confirmingDelete.id) {
        navigate('/reviews/questions', { replace: true })
      }
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const ordered = useMemo(
    () =>
      [...rows].sort((a, b) => {
        const la = (a.content?.label || a.slug).toLowerCase()
        const lb = (b.content?.label || b.slug).toLowerCase()
        return la.localeCompare(lb)
      }),
    [rows],
  )

  const needle = query.trim().toLowerCase()
  const visible = useMemo(() => {
    if (!needle) return ordered
    return ordered.filter((r) => {
      const label = (r.content?.label || '').toLowerCase()
      return (
        r.slug.toLowerCase().includes(needle) ||
        label.includes(needle)
      )
    })
  }, [ordered, needle])

  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load review questions"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }

  return (
    <>
      <DetailPageLayout
        leftSidebar={
          <aside className="flex w-80 shrink-0 flex-col border-r bg-card">
            <div className="flex flex-col gap-3 border-b px-4 py-3">
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-baseline gap-2">
                  <h2 className="text-sm font-semibold tracking-tight">Review questions</h2>
                  <span className="text-xs text-muted-foreground">{rows.length}</span>
                </div>
                {canManage && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-7 gap-1 px-2 text-xs"
                    onClick={() => setCreateOpen(true)}
                  >
                    <PlusIcon className="size-3.5" />
                    New
                  </Button>
                )}
              </div>
              <div className="relative">
                <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search questions…"
                  className="h-8 pl-8 text-xs"
                />
              </div>
            </div>

            {loading && rows.length === 0 ? (
              <p className="px-4 py-3 text-xs text-muted-foreground">Loading…</p>
            ) : rows.length === 0 ? (
              <div className="flex flex-1 flex-col items-center justify-center gap-2 px-4 py-12 text-center">
                <MessageCircleQuestionIcon className="size-6 text-muted-foreground/60" />
                <p className="text-sm font-medium">No questions yet</p>
                {canManage && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setCreateOpen(true)}
                  >
                    <PlusIcon className="size-3.5" />
                    Create your first
                  </Button>
                )}
              </div>
            ) : visible.length === 0 ? (
              <p className="px-4 py-6 text-center text-xs text-muted-foreground">
                No matches for &ldquo;{query}&rdquo;
              </p>
            ) : (
              <ul className="scrollbar-accent flex-1 overflow-y-auto py-1">
                {visible.map((row) => (
                  <li key={row.id}>
                    <SidebarRow
                      row={row}
                      active={row.id === activeID}
                      canManage={canManage}
                      onDelete={() => {
                        setDeleteError(null)
                        setConfirmingDelete(row)
                      }}
                    />
                  </li>
                ))}
              </ul>
            )}
          </aside>
        }
      >
        {activeID ? (
          <ReviewQuestionDetail questionId={activeID} />
        ) : (
          <EmptyState
            canManage={canManage}
            onNew={() => setCreateOpen(true)}
          />
        )}
      </DetailPageLayout>

      <ReviewQuestionFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        attribute={null}
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
              Delete &quot;{confirmingDelete?.content?.label || confirmingDelete?.slug}&quot;?
            </AlertDialogTitle>
            <AlertDialogDescription>
              The question is unassigned from every product type. Existing answers on past
              reviews are deleted along with it; existing reviews keep their headers but
              lose this question's answer row. This is permanent.
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
              {deleting ? (
                'Deleting…'
              ) : (
                <>
                  <CheckIcon className="size-4" />
                  Delete question
                </>
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function SidebarRow({
  row,
  active,
  canManage,
  onDelete,
}: {
  row: ReviewQuestionFieldsFragment
  active: boolean
  canManage: boolean
  onDelete: () => void
}) {
  const label = row.content?.label || row.slug
  const typeShort = row.inputType.toLowerCase().replace('_', ' ')
  return (
    <div className="group/row relative">
      <Link
        to={`/reviews/questions/${row.id}`}
        className={`mx-1 flex items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors ${
          active
            ? 'bg-primary/10 text-foreground'
            : 'text-foreground/90 hover:bg-muted/60'
        }`}
      >
        <div className="grid size-9 shrink-0 place-items-center rounded-md bg-muted">
          <MessageCircleQuestionIcon
            className={`size-4 ${active ? 'text-primary' : 'text-muted-foreground'}`}
            aria-hidden
          />
        </div>
        <div className="min-w-0 flex-1">
          <p className={`truncate text-sm ${active ? 'font-medium' : ''}`}>
            {label}
            {row.isRequired && (
              <span className="ml-1.5 text-[10px] font-normal text-muted-foreground">
                · required
              </span>
            )}
          </p>
          <p className="truncate font-mono text-[10px] text-muted-foreground">
            {row.slug} · {typeShort} · {row.allContent.length}{' '}
            {row.allContent.length === 1 ? 'locale' : 'locales'}
          </p>
        </div>
      </Link>
      {canManage && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute right-2 top-1/2 size-7 -translate-y-1/2 text-muted-foreground opacity-0 transition-opacity hover:bg-muted/60 group-hover/row:opacity-100 data-[state=open]:opacity-100"
              aria-label={`Actions for ${label}`}
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
              }}
            >
              <MoreHorizontalIcon className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem asChild>
              <Link to={`/reviews/questions/${row.id}`}>Open</Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={onDelete}>
              <Trash2Icon className="size-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  )
}

function EmptyState({
  canManage,
  onNew,
}: {
  canManage: boolean
  onNew: () => void
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-6 py-12 text-center">
      <MessageCircleQuestionIcon className="size-8 text-muted-foreground/60" />
      <div className="grid gap-1">
        <p className="text-sm font-medium">Select a question</p>
        <p className="max-w-sm text-xs text-muted-foreground">
          Pick a review question from the sidebar to edit it, or create a new one.
        </p>
      </div>
      {canManage && (
        <Button size="sm" variant="outline" onClick={onNew}>
          <PlusIcon className="size-3.5" />
          New question
        </Button>
      )}
    </div>
  )
}
