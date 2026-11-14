import { useMemo, useState } from 'react'
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
import { webformFileUrl } from '@/lib/api-endpoint'
import { useDownload } from '@/lib/authed-download'
import { formatBytes } from '@/lib/format'
import { WebFormDocument, WebFormSubmissionsDocument } from './WebForms.gql'
import type { Field } from './types'

type SubmissionFile = { fieldKey: string; name: string; size: number }
type SubmissionRow = {
  id: string
  answers: Record<string, unknown>
  files: SubmissionFile[]
  createdAt: string
}

// What a shopper uploaded with a submission. The API hands these out as loose JSON, so anything misshapen is dropped
function filesOf(raw: unknown): SubmissionFile[] {
  if (!Array.isArray(raw)) return []
  return raw.flatMap((f: unknown) => {
    const { fieldKey, name, size } = (f ?? {}) as Record<string, unknown>
    return typeof fieldKey === 'string' && fieldKey ? [{ fieldKey, name: typeof name === 'string' && name ? name : fieldKey, size: typeof size === 'number' ? size : 0 }] : []
  })
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
        files: filesOf(s.files),
        createdAt: s.createdAt as string,
      })),
    [data],
  )

  const [fileError, setFileError] = useState<string | null>(null)
  const download = useDownload((e, name) => setFileError(`${name}: ${e instanceof Error ? e.message : "couldn't be downloaded"}`))
  const hasFiles = rows.some((r) => r.files.length > 0)

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
    for (const f of inputFields.filter((x) => x.type !== 'file').slice(0, 6)) {
      cols.push({
        id: `f_${f.key}`,
        header: ({ column }) => <DataTableColumnHeader column={column} title={f.label || f.key} />,
        cell: ({ row }) => (
          <span className="line-clamp-2 max-w-[240px] text-sm">{stringify(row.original.answers[f.key])}</span>
        ),
      })
    }
    if (hasFiles) {
      cols.push({
        id: 'files',
        header: 'Files',
        cell: ({ row }) => (
          <div className="flex flex-wrap gap-1">
            {row.original.files.map((f) => (
              <Button
                key={f.fieldKey}
                type="button"
                variant="outline"
                size="xs"
                title={`Download ${f.name}`}
                disabled={download.busy === webformFileUrl(row.original.id, f.fieldKey)}
                onClick={() => {
                  setFileError(null)
                  download.start(webformFileUrl(row.original.id, f.fieldKey), f.name)
                }}
              >
                <DownloadIcon className="size-3" />
                <span className="max-w-40 truncate">{f.name}</span>
                {f.size > 0 ? <span className="text-muted-foreground">{formatBytes(f.size)}</span> : null}
              </Button>
            ))}
          </div>
        ),
      })
    }
    return cols
  }, [inputFields, hasFiles, download])

  function exportCsv() {
    const headers = ['submitted_at', ...inputFields.map((f) => f.key)]
    const lines = [headers.join(',')]
    for (const r of rows) {
      const cells = [new Date(r.createdAt).toISOString(), ...inputFields.map((f) => (f.type === 'file' ? (r.files.find((x) => x.fieldKey === f.key)?.name ?? '') : stringify(r.answers[f.key])))]
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
            {fileError ? (
              <Alert variant="destructive" className="mb-3">
                <AlertTitle>That file didn't download</AlertTitle>
                <AlertDescription>{fileError}</AlertDescription>
              </Alert>
            ) : null}
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
