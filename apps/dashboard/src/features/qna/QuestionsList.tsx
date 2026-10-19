import { useMutation, useQuery } from '@apollo/client/react'
import { useMemo, useState, type FormEvent } from 'react'
import type { ColumnDef } from '@/components/data-table/tanstack'

import { DataTable } from '@/components/data-table'
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
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { MoreHorizontalIcon } from 'lucide-react'

import {
  AnswerProductQuestionDocument,
  DeleteProductQuestionDocument,
  ProductQuestionsDocument,
  RejectProductQuestionDocument,
} from './Qna.gql'

type Row = {
  id: string
  authorName: string
  authorEmail: string
  question: string
  answer: string
  status: string
  createdAt: string
}

const STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive'> = {
  published: 'default',
  pending: 'secondary',
  rejected: 'destructive',
}

export function QuestionsList() {
  const [status, setStatus] = useState('pending')
  const { data, loading, refetch } = useQuery(ProductQuestionsDocument, {
    variables: { status: status === 'all' ? '' : status, limit: 100 },
  })
  const [answering, setAnswering] = useState<Row | null>(null)
  const [reject] = useMutation(RejectProductQuestionDocument)
  const [remove] = useMutation(DeleteProductQuestionDocument)

  const rows: Row[] = useMemo(
    () =>
      (data?.productQuestions ?? []).map((q) => ({
        id: q.id,
        authorName: q.authorName,
        authorEmail: q.authorEmail,
        question: q.question,
        answer: q.answer,
        status: q.status,
        createdAt: q.createdAt as string,
      })),
    [data],
  )

  const columns: ColumnDef<Row>[] = useMemo(
    () => [
      {
        accessorKey: 'question',
        header: 'Question',
        cell: ({ row }) => (
          <div className="max-w-xl">
            <p className="line-clamp-2">{row.original.question}</p>
            {row.original.answer && (
              <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                A: {row.original.answer}
              </p>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'authorName',
        header: 'From',
        cell: ({ row }) => (
          <div>
            <p>{row.original.authorName || '-'}</p>
            <p className="text-xs text-muted-foreground">
              {row.original.authorEmail}
            </p>
          </div>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => (
          <Badge variant={STATUS_VARIANT[row.original.status] ?? 'secondary'}>
            {row.original.status}
          </Badge>
        ),
      },
      {
        accessorKey: 'createdAt',
        header: 'Asked',
        cell: ({ row }) => new Date(row.original.createdAt).toLocaleDateString(),
      },
      {
        id: 'actions',
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button aria-label="Question actions" variant="ghost" size="icon" className="size-8">
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => setAnswering(row.original)}>
                {row.original.status === 'published' ? 'Edit answer' : 'Answer'}
              </DropdownMenuItem>
              {row.original.status !== 'rejected' && (
                <DropdownMenuItem
                  onSelect={() =>
                    void reject({ variables: { id: row.original.id } }).then(
                      () => refetch(),
                    )
                  }
                >
                  Reject
                </DropdownMenuItem>
              )}
              <DropdownMenuItem
                variant="destructive"
                onSelect={() =>
                  void remove({ variables: { id: row.original.id } }).then(() =>
                    refetch(),
                  )
                }
              >
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [reject, remove, refetch],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Product Q&A"
        meta="Shopper questions from product pages. Answering publishes the question; rejected questions never appear on the storefront."
        actions={
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="pending">Pending</SelectItem>
              <SelectItem value="published">Published</SelectItem>
              <SelectItem value="rejected">Rejected</SelectItem>
              <SelectItem value="all">All</SelectItem>
            </SelectContent>
          </Select>
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable columns={columns} data={rows} loading={loading} />
        </DetailPageSection>
      </DetailPageBody>

      <AnswerDialog
        question={answering}
        onClose={() => setAnswering(null)}
        onAnswered={() => void refetch()}
      />
    </DetailPageLayout>
  )
}

function AnswerDialog({
  question,
  onClose,
  onAnswered,
}: {
  question: Row | null
  onClose: () => void
  onAnswered: () => void
}) {
  const [answer, setAnswer] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submit, { loading }] = useMutation(AnswerProductQuestionDocument)

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!question) return
    setError(null)
    try {
      await submit({ variables: { id: question.id, answer: answer.trim() } })
      setAnswer('')
      onAnswered()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save answer.')
    }
  }

  return (
    <Dialog
      open={!!question}
      onOpenChange={(v) => {
        if (!v) {
          setAnswer('')
          setError(null)
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Answer question</DialogTitle>
          <DialogDescription className="line-clamp-3">
            {question?.question}
          </DialogDescription>
        </DialogHeader>
        <form id="qna-answer" onSubmit={onSubmit} className="grid gap-3">
          <Textarea
            required
            rows={5}
            placeholder="Write the public answer…"
            value={answer || question?.answer || ''}
            onChange={(e) => setAnswer(e.target.value)}
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
        </form>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="qna-answer" disabled={loading}>
            {loading ? 'Publishing…' : 'Publish answer'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
