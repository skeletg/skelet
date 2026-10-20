import { useQuery } from '@apollo/client/react'

import { Badge } from '@/components/ui/badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { ScheduledJobRunsDocument } from './Scheduler.gql'
import { formatTimestamp } from '@/lib/format'

function RunStatusBadge({ status }: { status: string }) {
  if (status === 'success') return <Badge variant="success">success</Badge>
  return <Badge variant="destructive">error</Badge>
}

/** RunHistorySheet shows a job's recent execution history (newest first) */
export function RunHistorySheet({
  jobId,
  jobName,
  onClose,
}: {
  jobId: string
  jobName: string
  onClose: () => void
}) {
  const { data, loading, error } = useQuery(ScheduledJobRunsDocument, {
    variables: { jobId, limit: 50 },
    fetchPolicy: 'cache-and-network',
  })
  const runs = data?.scheduledJobRuns ?? []

  return (
    <Sheet open onOpenChange={(o) => { if (!o) onClose() }}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>Run history - {jobName}</SheetTitle>
          <SheetDescription>Most recent 50 runs, newest first.</SheetDescription>
        </SheetHeader>

        <div className="grid gap-3 px-4 pb-6">
          {loading && runs.length === 0 ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : error ? (
            <p className="text-sm text-destructive">{error.message}</p>
          ) : runs.length === 0 ? (
            <p className="text-sm text-muted-foreground">No runs recorded yet.</p>
          ) : (
            runs.map((r, i) => (
              <div key={i} className="rounded-md border p-3 text-sm">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <RunStatusBadge status={r.status} />
                    {r.attempt > 1 && (
                      <Badge variant="outline" className="font-normal">attempt {r.attempt}</Badge>
                    )}
                  </div>
                  <span className="text-xs text-muted-foreground">{r.durationMs} ms</span>
                </div>
                <div className="mt-1.5 text-xs text-muted-foreground">
                  {formatTimestamp(r.startedAt)} → {formatTimestamp(r.finishedAt)}
                </div>
                {r.status === 'error' && r.error && (
                  <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-destructive/10 p-2 font-mono text-xs text-destructive">
                    {r.error}
                  </pre>
                )}
              </div>
            ))
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
