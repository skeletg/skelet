import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import {
  AssignReviewQuestionDocument,
  ProductTypeReviewQuestionsDocument,
  ReviewQuestionsDocument,
  UnassignReviewQuestionDocument,
} from '@/features/reviews/Reviews.gql'

// "Review questions" card on the ProductType detail page - Mirrors the
// attribute-assignment pattern: list of currently-assigned questions,
// a button that opens a small dialog to add one from the global pool,
// and a row-level remove action - Assignment + unassignment are
// idempotent in the backend (PRIMARY KEY swallows duplicates)

export function ProductTypeReviewsCard({
  productTypeId,
  canManage,
}: {
  productTypeId: string
  canManage: boolean
}) {
  const assignedQuery = useQuery(ProductTypeReviewQuestionsDocument, {
    variables: { id: productTypeId },
  })
  const allQuery = useQuery(ReviewQuestionsDocument)

  const refetch = [
    { query: ProductTypeReviewQuestionsDocument, variables: { id: productTypeId } },
  ]
  const [assign, { loading: assigning }] = useMutation(AssignReviewQuestionDocument, {
    refetchQueries: refetch,
  })
  const [unassign, { loading: unassigning }] = useMutation(UnassignReviewQuestionDocument, {
    refetchQueries: refetch,
  })

  const [assignOpen, setAssignOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const assigned = useMemo(() => assignedQuery.data?.productType?.reviewQuestions ?? [], [assignedQuery.data])
  const all = useMemo(() => allQuery.data?.reviewQuestions ?? [], [allQuery.data])

  const assignedIDs = useMemo(() => new Set(assigned.map((q) => q.id)), [assigned])
  const assignable = useMemo(
    () => all.filter((q) => !assignedIDs.has(q.id)),
    [all, assignedIDs],
  )

  async function onRemove(questionId: string) {
    setError(null)
    try {
      await unassign({ variables: { productTypeId, questionId } })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unassign failed.')
    }
  }

  return (
    <section className="grid gap-4 px-6 py-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="grid gap-1">
          <h3 className="text-lg font-semibold tracking-tight">
            <span className="mr-1.5 text-primary">#</span>
            Review questions
          </h3>
          <p className="text-sm text-muted-foreground">
            Customers leaving a review for products of this type answer
            each question in order. Curate the question library on the{' '}
            <Link to="/reviews/questions" className="underline underline-offset-2">
              Review questions
            </Link>{' '}
            page.
          </p>
        </div>
        {canManage ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setAssignOpen(true)}
            disabled={assignable.length === 0}
          >
            <PlusIcon className="size-4" />
            Assign question
          </Button>
        ) : null}
      </div>
      <div className="grid gap-4">
        {error ? (
          <Alert variant="destructive">
            <AlertTitle>Couldn't update</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {assigned.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No questions assigned yet. The review form will only
            collect a free-text body until you assign at least one.
          </p>
        ) : (
          <ul className="grid gap-2">
            {assigned.map((q) => (
              <li
                key={q.id}
                className="flex items-center justify-between gap-3 rounded-md border p-2 text-sm"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span className="w-6 shrink-0 text-xs text-muted-foreground tabular-nums">
                    {q.sortOrder}
                  </span>
                  <Link
                    to={`/reviews/questions/${q.id}`}
                    className="truncate font-medium hover:underline"
                  >
                    {q.content?.label || q.slug}
                  </Link>
                  <Badge variant="outline" className="font-mono text-[0.65rem] uppercase">
                    {q.inputType.toLowerCase().replace('_', ' ')}
                  </Badge>
                  {q.isRequired ? <Badge variant="secondary">Required</Badge> : null}
                </div>
                {canManage ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => void onRemove(q.id)}
                    disabled={unassigning}
                    title="Unassign"
                  >
                    <Trash2Icon className="size-4" />
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      <AssignQuestionDialog
        open={assignOpen}
        onOpenChange={setAssignOpen}
        assignable={assignable}
        existingSortOrders={assigned.map((q) => q.sortOrder)}
        busy={assigning}
        onSubmit={async (questionId, sortOrder) => {
          setError(null)
          try {
            await assign({
              variables: { input: { productTypeId, questionId, sortOrder } },
            })
            setAssignOpen(false)
          } catch (err) {
            setError(err instanceof Error ? err.message : 'Assign failed.')
          }
        }}
      />
    </section>
  )
}

function AssignQuestionDialog({
  open,
  onOpenChange,
  assignable,
  existingSortOrders,
  busy,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  assignable: { id: string; slug: string; content?: { label: string } | null }[]
  existingSortOrders: number[]
  busy: boolean
  onSubmit: (questionId: string, sortOrder: number) => Promise<void>
}) {
  const nextSort = existingSortOrders.length === 0 ? 0 : Math.max(...existingSortOrders) + 1
  const [questionId, setQuestionId] = useState('')
  const [sortOrder, setSortOrder] = useState(nextSort)

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          setQuestionId('')
          setSortOrder(nextSort)
        }
        onOpenChange(o)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Assign review question</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label>Question</Label>
            <Select value={questionId} onValueChange={setQuestionId}>
              <SelectTrigger>
                <SelectValue placeholder="Pick a question…" />
              </SelectTrigger>
              <SelectContent>
                {assignable.map((q) => (
                  <SelectItem key={q.id} value={q.id}>
                    {q.content?.label || q.slug}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="rq-sort">Sort order</Label>
            <Input
              id="rq-sort"
              type="number"
              value={sortOrder}
              onChange={(e) => setSortOrder(Number(e.target.value))}
              className="max-w-[120px]"
            />
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            Cancel
          </Button>
          <Button
            disabled={busy || !questionId}
            onClick={() => void onSubmit(questionId, sortOrder)}
          >
            {busy ? 'Saving…' : 'Assign'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
