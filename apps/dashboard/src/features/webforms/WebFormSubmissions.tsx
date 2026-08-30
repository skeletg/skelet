import { useMemo } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { DownloadIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { WebFormDocument, WebFormSubmissionsDocument } from './WebForms.gql'
import type { Field } from './types'

type SubmissionRow = {
  id: string
  answers: Record<string, unknown>
  files: unknown
  createdAt: string
}

function stringify(v: unknown): string {
  if (v == null) return ''
  if (typeof v === 'boolean') return v ? 'Yes' : 'No'
  if (Array.isArray(v)) return v.map(stringify).filter(Boolean).join(', ')
  if (typeof v === 'object') {
    // matrix (row=selection) or repeater instance (key=value)
    return Object.entries(v as Record<string, unknown>)
      .map(([k, val]) => `${k}=${stringify(val)}`)
      .join('; ')
  }
  return String(v)
}

export function WebFormSubmissions() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { id } = useParams<{ id: string }>()
  const formQuery = useQuery(WebFormDocument, { variables: { id: id ?? '' }, skip: !id })
  const { data, loading, error } = useQuery(WebFormSubmissionsDocument, {
    variables: { formId: id ?? '', limit: 200, offset: 0 },
    skip: !id,
    fetchPolicy: 'cache-and-network',
  })

  const form = formQuery.data?.webForm
  const fields: Field[] = Array.isArray(form?.fields) ? (form!.fields as Field[]) : []
  const inputFields = fields.filter((f) => f.key && !['heading', 'paragraph', 'divider'].includes(f.type))

  const rows = useMemo<SubmissionRow[]>(
    () =>
      (data?.webFormSubmissions ?? []).map((s) => ({
        id: s.id,
        answers: (s.answers as Record<string, unknown>) ?? {},
        files: s.files,
        createdAt: s.createdAt as string,
      })),
    [data],
  )

  const columns = useMemo<ColumnDef<SubmissionRow>[]>(() => {
    const cols: ColumnDef<SubmissionRow>[] = [
      {
        accessorKey: 'createdAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Submitted" />,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-sm text-muted-foreground">
            {new Date(row.original.createdAt).toLocaleString()}
          </span>
        ),
      },
    ]
    for (const f of inputFields.slice(0, 6)) {
      cols.push({
        id: `f_${f.key}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title={f.label || f.key} />,
        cell: ({ row }) => (
          <span className="line-clamp-2 max-w-[240px] text-sm">{stringify(row.original.answers[f.key])}</span>
        ),
      })
    }
    return cols
  }, [inputFields])

  function exportCsv() {
    const headers = ['submitted_at', ...inputFields.map((f) => f.key)]
    const lines = [headers.join(',')]
    for (const r of rows) {
      const cells = [new Date(r.createdAt).toISOString(), ...inputFields.map((f) => stringify(r.answers[f.key]))]
      lines.push(cells.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(','))
    }
    const blob = new Blob([lines.join('\n')], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${form?.slug ?? 'form'}-submissions.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo={`/cms/forms/${id}`}
        backLabel="Form"
        title={form ? `${form.name} - submissions` : 'Submissions'}
        actions={
          <Button variant="outline" size="sm" onClick={exportCsv} disabled={rows.length === 0}>
            <DownloadIcon className="size-4" /> Export CSV
          </Button>
        }
      />
      <DetailPageBody>
        {error ? (
          <DetailPageSection>
            <Alert variant="destructive">
              <AlertTitle>Couldn't load submissions</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          </DetailPageSection>
        ) : (
          <DetailPageSection>
            <DataTable
              urlState={urlState}
              columns={columns}
              data={rows}
              loading={loading}
              getRowId={(r) => r.id}
              emptyState="No submissions yet."
              searchPlaceholder="Search submissions…"
            />
            <p className="mt-2 text-xs text-muted-foreground">
              Showing up to 200 most recent. <Link to={`/cms/forms/${id}`} className="hover:underline">Edit form →</Link>
            </p>
          </DetailPageSection>
        )}
      </DetailPageBody>
    </DetailPageLayout>
  )
}
