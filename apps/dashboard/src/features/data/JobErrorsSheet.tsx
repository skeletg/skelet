import { useQuery } from '@apollo/client/react'

import { DownloadIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { toCsv, download } from '@/components/data-table'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { LoadingState } from '@/components/ui/loading-state'
import { DataJobErrorsDocument } from './Data.gql'
import type { DataJob } from './jobs'

// The per-row report. Line numbers are the file's own, so a fix is a jump in the spreadsheet rather than a hunt
export function JobErrorsSheet({ job, onClose }: { job: DataJob; onClose: () => void }) {
  const { data, loading } = useQuery(DataJobErrorsDocument, {
    variables: { id: job.id, limit: 500 },
    fetchPolicy: 'cache-and-network',
  })
  const rows = data?.dataJobErrors.items ?? []
  const total = data?.dataJobErrors.totalCount ?? 0

  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="flex w-full flex-col overflow-hidden sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>Rows that failed - {job.filename || job.resourceLabel}</SheetTitle>
          <SheetDescription>
            {total.toLocaleString()} of {job.totalRows.toLocaleString()} rows.
            {job.errorsTruncated ? ' Only the first batch was kept.' : ''}
          </SheetDescription>
        </SheetHeader>

        <div className="px-4">
          {/* The fix-and-reupload loop: take this alongside the source file rather than transcribing the sheet */}
          <Button
            variant="outline"
            size="sm"
            disabled={rows.length === 0}
            onClick={() =>
              download(
                toCsv(
                  ['line', 'row', 'problem'],
                  rows.map((e) => [String(e.line), e.ref, e.message]),
                ),
                `${job.resource}-errors.csv`,
              )
            }
          >
            <DownloadIcon className="mr-2 size-4" />
            Download as CSV
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto px-4 pb-6">
          {loading && rows.length === 0 ? (
            <LoadingState />
          ) : (
            <div className="overflow-hidden rounded-md border">
              <table className="w-full text-sm">
                <thead className="border-b bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 font-medium">Line</th>
                    <th className="px-3 py-2 font-medium">Row</th>
                    <th className="px-3 py-2 font-medium">Problem</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {rows.map((e, i) => (
                    <tr key={`${e.line}-${i}`} className="align-top">
                      <td className="px-3 py-2 font-mono text-xs tabular-nums">{e.line}</td>
                      <td className="px-3 py-2 font-mono text-xs">{e.ref || '-'}</td>
                      <td className="px-3 py-2">{e.message}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
