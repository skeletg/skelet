import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  HistoryIcon,
  PauseIcon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  SettingsIcon,
  Trash2Icon,
} from 'lucide-react'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import type { ScheduledJobsQuery } from './Scheduler.gql'
import {
  CreateWebhookJobDocument,
  DeleteScheduledJobDocument,
  RunScheduledJobNowDocument,
  ScheduledJobsDocument,
  UpdateScheduledJobDocument,
  UpdateWebhookJobDocument,
  WebhookJobConfigDocument,
} from './Scheduler.gql'
import { RunHistorySheet } from './RunHistorySheet'
import { useSchedulerPaused } from './scheduler-config'
import { SchedulerSettings } from './SchedulerSettings'
import { HeadersEditor, RetryAlertFields, SchedulePicker, type Header } from './SchedulerWidgets'
import { formatTimestamp } from '@/lib/format'

type Job = ScheduledJobsQuery['scheduledJobs'][number]

type ScheduleEdit = {
  schedule: string
  enabled: boolean
  timezone: string
  maxAttempts: number
  retryBackoffSeconds: number
  alertAfterFailures: number
  timeoutSeconds: number
}

function StatusBadge({ status }: { status: string }) {
  if (status === 'success') return <Badge variant="success">success</Badge>
  if (status === 'error') return <Badge variant="destructive">error</Badge>
  return <span className="text-muted-foreground">-</span>
}

export function ScheduledJobsList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('scheduler.manage')
  const paused = useSchedulerPaused()

  const { data, loading, error } = useQuery(ScheduledJobsDocument)
  const jobs = useMemo(() => data?.scheduledJobs ?? [], [data])

  const refetch = [{ query: ScheduledJobsDocument }]
  const [updateJob] = useMutation(UpdateScheduledJobDocument, { refetchQueries: refetch })
  const [runNow] = useMutation(RunScheduledJobNowDocument, { refetchQueries: refetch })
  const [createWebhook] = useMutation(CreateWebhookJobDocument, { refetchQueries: refetch })
  const [updateWebhook] = useMutation(UpdateWebhookJobDocument, { refetchQueries: refetch })
  const [deleteJob] = useMutation(DeleteScheduledJobDocument, { refetchQueries: refetch })

  const [editing, setEditing] = useState<Job | null>(null)
  const [editingWebhook, setEditingWebhook] = useState<Job | null>(null)
  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<Job | null>(null)
  const [history, setHistory] = useState<Job | null>(null)
  const [errorView, setErrorView] = useState<Job | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const onRunNow = useCallback(
    async (job: Job) => {
      setActionError(null)
      try {
        await runNow({ variables: { id: job.id } })
      } catch (err) {
        setActionError(err instanceof Error ? err.message : 'Run failed.')
      }
    },
    [runNow],
  )

  async function onConfirmDelete() {
    if (!confirmDelete) return
    setActionError(null)
    try {
      await deleteJob({ variables: { id: confirmDelete.id } })
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<Job>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Job" />,
        cell: ({ row }) => {
          const job = row.original
          return (
            <>
              <div className="font-medium">{job.name}</div>
              {job.description && <div className="text-xs text-muted-foreground">{job.description}</div>}
              {job.webhook && (
                <code className="block max-w-md truncate font-mono text-xs text-muted-foreground">
                  {job.webhook.method} {job.webhook.host}
                </code>
              )}
            </>
          )
        },
      },
      {
        accessorKey: 'kind',
        header: 'Kind',
        size: 96,
        cell: ({ row }) => {
          const job = row.original
          return (
            <Badge variant={job.kind === 'webhook' ? 'secondary' : 'outline'} className="font-normal">{job.kind}</Badge>
          )
        },
      },
      {
        accessorKey: 'schedule',
        header: 'Schedule',
        size: 160,
        cell: ({ row }) => {
          const job = row.original
          return (
            <>
              <code className="font-mono text-xs">{job.schedule}</code>
              {job.timezone && <div className="text-[11px] text-muted-foreground">{job.timezone}</div>}
              {job.maxAttempts > 1 && (
                <div className="text-[11px] text-muted-foreground">retries: {job.maxAttempts}</div>
              )}
            </>
          )
        },
      },
      {
        accessorKey: 'enabled',
        header: 'Enabled',
        size: 80,
        cell: ({ row }) => (
          row.original.enabled
            ? <Badge variant="outline" className="font-normal">on</Badge>
            : <Badge variant="outline" className="font-normal text-muted-foreground">off</Badge>
        ),
      },
      {
        accessorKey: 'nextRunAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Next run" />,
        size: 176,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{row.original.enabled ? formatTimestamp(row.original.nextRunAt) : '-'}</span>
        ),
      },
      {
        accessorKey: 'lastRunAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Last run" />,
        size: 224,
        cell: ({ row }) => {
          const job = row.original
          return (
            <div className="text-xs">
              <div className="flex items-center gap-2">
                <StatusBadge status={job.lastStatus} />
                {job.lastRunAt && <span className="text-muted-foreground">{formatTimestamp(job.lastRunAt)}</span>}
              </div>
              {job.currentAttempt > 0 && (
                <div className="mt-0.5 text-warning">retrying - attempt {job.currentAttempt + 1}</div>
              )}
              {job.lastStatus === 'error' && job.lastError && (
                <button
                  type="button"
                  className="mt-0.5 block max-w-xs truncate text-left text-destructive underline-offset-2 hover:underline"
                  title="View full error"
                  onClick={() => setErrorView(job)}
                >
                  {job.lastError}
                </button>
              )}
            </div>
          )
        },
      },
      {
        id: 'actions',
        size: 208,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => {
          const job = row.original
          return (
            <div className="flex items-center justify-end gap-1">
              <Button variant="ghost" size="icon" className="size-7" title="Run history" onClick={() => setHistory(job)}>
                <HistoryIcon className="size-3.5" />
              </Button>
              {canManage && (
                <>
                  <Button variant="ghost" size="icon" className="size-7" title="Run now" onClick={() => onRunNow(job)}>
                    <PlayIcon className="size-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon" className="size-7" title="Edit schedule" onClick={() => setEditing(job)}>
                    <SettingsIcon className="size-3.5" />
                  </Button>
                  {job.kind === 'webhook' && (
                    <Button variant="ghost" size="icon" className="size-7" title="Edit request" onClick={() => setEditingWebhook(job)}>
                      <PencilIcon className="size-3.5" />
                    </Button>
                  )}
                  {job.kind === 'webhook' && (
                    <Button variant="ghost" size="icon" className="size-7 text-destructive hover:bg-destructive/10 hover:text-destructive" title="Delete" onClick={() => setConfirmDelete(job)}>
                      <Trash2Icon className="size-3.5" />
                    </Button>
                  )}
                </>
              )}
            </div>
          )
        },
      },
    ],
    [canManage, onRunNow],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Scheduled jobs"
        meta="Recurring tasks. Code jobs are built in (schedule + on/off configurable); webhook jobs fire an HTTP call on their schedule."
        actions={
          canManage && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon className="size-4" /> New webhook job
            </Button>
          )
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          {paused && (
            <Alert>
              <PauseIcon className="size-4" />
              <AlertTitle>Scheduler paused</AlertTitle>
              <AlertDescription>
                The master switch is off - no jobs are claimed or run. Re-enable it in Scheduler settings below.
              </AlertDescription>
            </Alert>
          )}

          <SchedulerSettings />

          {actionError && (
            <Alert variant="destructive">
              <AlertTitle>Action failed</AlertTitle>
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          )}

          <DataTable
            urlState={urlState}
            columns={columns}
            data={jobs}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(job) => job.id}
            emptyState="No scheduled jobs."
            searchPlaceholder="Search jobs…"
          />
        </DetailPageSection>
      </DetailPageBody>

      {editing && (
        <EditScheduleDialog
          job={editing}
          onClose={() => setEditing(null)}
          onSave={async (edit) => {
            setActionError(null)
            try {
              await updateJob({
                variables: {
                  input: {
                    id: editing.id,
                    schedule: edit.schedule,
                    enabled: edit.enabled,
                    timezone: edit.timezone,
                    maxAttempts: edit.maxAttempts,
                    retryBackoffSeconds: edit.retryBackoffSeconds,
                    alertAfterFailures: edit.alertAfterFailures,
                    timeoutSeconds: edit.timeoutSeconds,
                  },
                },
              })
              setEditing(null)
            } catch (err) {
              setActionError(err instanceof Error ? err.message : 'Save failed.')
              throw err
            }
          }}
        />
      )}

      {editingWebhook && (
        <WebhookEditDialog
          job={editingWebhook}
          onClose={() => setEditingWebhook(null)}
          onSave={async (input) => {
            setActionError(null)
            try {
              await updateWebhook({ variables: { input: { id: editingWebhook.id, ...input } } })
              setEditingWebhook(null)
            } catch (err) {
              setActionError(err instanceof Error ? err.message : 'Save failed.')
              throw err
            }
          }}
        />
      )}

      {creating && (
        <CreateWebhookDialog
          onClose={() => setCreating(false)}
          onCreate={async (input) => {
            setActionError(null)
            try {
              await createWebhook({ variables: { input } })
              setCreating(false)
            } catch (err) {
              setActionError(err instanceof Error ? err.message : 'Create failed.')
              throw err
            }
          }}
        />
      )}

      {history && (
        <RunHistorySheet jobId={history.id} jobName={history.name} onClose={() => setHistory(null)} />
      )}

      <Dialog open={!!errorView} onOpenChange={(o) => { if (!o) setErrorView(null) }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Last error - {errorView?.name}</DialogTitle>
            <DialogDescription>{formatTimestamp(errorView?.lastRunAt)}</DialogDescription>
          </DialogHeader>
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded bg-destructive/10 p-3 font-mono text-xs text-destructive">
            {errorView?.lastError}
          </pre>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!confirmDelete}
        onOpenChange={(o) => { if (!o) setConfirmDelete(null) }}
        title={<>Delete “{confirmDelete?.name}”?</>}
        description="This removes the webhook job permanently."
        confirmLabel="Delete"
        // Not awaited: the dialog closes at once and a failure shows on the page, as it always has
        onConfirm={() => void onConfirmDelete()}
      />
    </DetailPageLayout>
  )
}

function EditScheduleDialog({
  job,
  onClose,
  onSave,
}: {
  job: Job
  onClose: () => void
  onSave: (edit: ScheduleEdit) => Promise<void>
}) {
  const [schedule, setSchedule] = useState(job.schedule)
  const [enabled, setEnabled] = useState(job.enabled)
  const [timezone, setTimezone] = useState(job.timezone)
  const [maxAttempts, setMaxAttempts] = useState(job.maxAttempts)
  const [retryBackoffSeconds, setRetryBackoffSeconds] = useState(job.retryBackoffSeconds)
  const [alertAfterFailures, setAlertAfterFailures] = useState(job.alertAfterFailures)
  const [timeoutSeconds, setTimeoutSeconds] = useState(job.timeoutSeconds)
  const [saving, setSaving] = useState(false)
  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose() }}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit “{job.name}”</DialogTitle>
          <DialogDescription>Schedule, timezone, retries and failure alerting.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <SchedulePicker
            schedule={schedule}
            onScheduleChange={setSchedule}
            timezone={timezone}
            onTimezoneChange={setTimezone}
          />
          <RetryAlertFields
            maxAttempts={maxAttempts}
            onMaxAttempts={setMaxAttempts}
            retryBackoffSeconds={retryBackoffSeconds}
            onRetryBackoffSeconds={setRetryBackoffSeconds}
            alertAfterFailures={alertAfterFailures}
            onAlertAfterFailures={setAlertAfterFailures}
          />
          <div className="grid gap-1.5">
            <Label htmlFor="timeout-seconds">Run timeout (seconds)</Label>
            <Input
              id="timeout-seconds"
              type="number"
              min={1}
              value={timeoutSeconds}
              onChange={(e) => setTimeoutSeconds(Math.max(1, Number(e.target.value) || 1))}
            />
            <span className="text-xs text-muted-foreground">
              The run is cancelled past this - raise it for long jobs like a backup or reindex (default 300)
            </span>
          </div>
          <div className="flex items-center justify-between rounded-md border p-3">
            <Label htmlFor="enabled">Enabled</Label>
            <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button
            disabled={saving || schedule.trim() === ''}
            onClick={async () => {
              setSaving(true)
              try {
                await onSave({ schedule: schedule.trim(), enabled, timezone, maxAttempts, retryBackoffSeconds, alertAfterFailures, timeoutSeconds })
              } catch {
                // surfaced by parent
              } finally {
                setSaving(false)
              }
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type CreateInput = {
  name: string
  schedule: string
  timezone?: string | null
  maxAttempts?: number | null
  retryBackoffSeconds?: number | null
  alertAfterFailures?: number | null
  url: string
  method?: string | null
  body?: string | null
  secret?: string | null
  headers?: Header[] | null
}

function CreateWebhookDialog({
  onClose,
  onCreate,
}: {
  onClose: () => void
  onCreate: (input: CreateInput) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [schedule, setSchedule] = useState('@every 1h')
  const [timezone, setTimezone] = useState('')
  const [maxAttempts, setMaxAttempts] = useState(1)
  const [retryBackoffSeconds, setRetryBackoffSeconds] = useState(60)
  const [alertAfterFailures, setAlertAfterFailures] = useState(0)
  const [url, setUrl] = useState('')
  const [method, setMethod] = useState('POST')
  const [body, setBody] = useState('')
  const [secret, setSecret] = useState('')
  const [headers, setHeaders] = useState<Header[]>([])
  const [saving, setSaving] = useState(false)
  const valid = name.trim() !== '' && schedule.trim() !== '' && url.trim() !== ''
  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose() }}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>New webhook job</DialogTitle>
          <DialogDescription>Fires an HTTP request on its schedule. The body is signed (HMAC-SHA256, header X-Job-Signature-256) when a secret is set.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-2">
            <Label htmlFor="w-name">Name</Label>
            <Input id="w-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="nightly-report" />
          </div>
          <SchedulePicker
            schedule={schedule}
            onScheduleChange={setSchedule}
            timezone={timezone}
            onTimezoneChange={setTimezone}
          />
          <div className="grid grid-cols-[6rem_1fr] gap-2">
            <div className="grid gap-2">
              <Label htmlFor="w-method">Method</Label>
              <Input id="w-method" value={method} onChange={(e) => setMethod(e.target.value.toUpperCase())} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="w-url">URL</Label>
              <Input id="w-url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://hooks.example.com/job" />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="w-body">Body (optional)</Label>
            <Textarea id="w-body" value={body} onChange={(e) => setBody(e.target.value)} placeholder='{"event":"nightly"}' rows={3} className="font-mono text-sm" />
          </div>
          <HeadersEditor headers={headers} onChange={setHeaders} />
          <div className="grid gap-2">
            <Label htmlFor="w-secret">Signing secret (optional)</Label>
            <Input id="w-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} />
          </div>
          <RetryAlertFields
            maxAttempts={maxAttempts}
            onMaxAttempts={setMaxAttempts}
            retryBackoffSeconds={retryBackoffSeconds}
            onRetryBackoffSeconds={setRetryBackoffSeconds}
            alertAfterFailures={alertAfterFailures}
            onAlertAfterFailures={setAlertAfterFailures}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button
            disabled={saving || !valid}
            onClick={async () => {
              setSaving(true)
              try {
                await onCreate({
                  name: name.trim(),
                  schedule: schedule.trim(),
                  timezone: timezone || null,
                  maxAttempts,
                  retryBackoffSeconds,
                  alertAfterFailures,
                  url: url.trim(),
                  method: method.trim() || 'POST',
                  body: body || null,
                  secret: secret || null,
                  headers: headers.filter((h) => h.name.trim() !== ''),
                })
              } catch {
                // error surfaced by parent
              } finally {
                setSaving(false)
              }
            }}
          >
            {saving ? 'Creating…' : 'Create'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type WebhookEditSave = {
  description?: string | null
  url: string
  method?: string | null
  body?: string | null
  secret?: string | null
  headers?: Header[] | null
}

function WebhookEditDialog({
  job,
  onClose,
  onSave,
}: {
  job: Job
  onClose: () => void
  onSave: (input: WebhookEditSave) => Promise<void>
}) {
  const { data, loading } = useQuery(WebhookJobConfigDocument, {
    variables: { id: job.id },
    fetchPolicy: 'network-only',
  })
  const cfg = data?.webhookJobConfig

  const [description, setDescription] = useState(job.description)
  const [url, setUrl] = useState('')
  const [method, setMethod] = useState('POST')
  const [body, setBody] = useState('')
  const [secret, setSecret] = useState('')
  const [headers, setHeaders] = useState<Header[]>([])
  const [saving, setSaving] = useState(false)
  const [seeded, setSeeded] = useState(false)

  // Seed once from the loaded config so a save round-trips body/headers instead of wiping them (state adjustment during
  // render; the `seeded` flag guarantees a single pass)
  if (cfg && !seeded) {
    setSeeded(true)
    setUrl(cfg.url)
    setMethod(cfg.method)
    setBody(cfg.body)
    setHeaders(cfg.headers.map((h) => ({ name: h.name, value: h.value })))
  }

  return (
    <Dialog open onOpenChange={(o) => { if (!o) onClose() }}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit request - {job.name}</DialogTitle>
          <DialogDescription>
            URL, method, body and headers. Leave the secret blank to keep the current one{cfg?.hasSecret ? '' : ' (none set)'}.
          </DialogDescription>
        </DialogHeader>
        {loading && !cfg ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <div className="grid gap-3">
            <div className="grid gap-2">
              <Label htmlFor="we-desc">Description</Label>
              <Input id="we-desc" value={description} onChange={(e) => setDescription(e.target.value)} />
            </div>
            <div className="grid grid-cols-[6rem_1fr] gap-2">
              <div className="grid gap-2">
                <Label htmlFor="we-method">Method</Label>
                <Input id="we-method" value={method} onChange={(e) => setMethod(e.target.value.toUpperCase())} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="we-url">URL</Label>
                <Input id="we-url" value={url} onChange={(e) => setUrl(e.target.value)} />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="we-body">Body</Label>
              <Textarea id="we-body" value={body} onChange={(e) => setBody(e.target.value)} rows={3} className="font-mono text-sm" />
            </div>
            <HeadersEditor headers={headers} onChange={setHeaders} />
            <div className="grid gap-2">
              <Label htmlFor="we-secret">{cfg?.hasSecret ? 'Rotate signing secret' : 'Signing secret'} (optional)</Label>
              <Input id="we-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={cfg?.hasSecret ? '•••••• (unchanged)' : ''} />
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button
            disabled={saving || !seeded || url.trim() === ''}
            onClick={async () => {
              setSaving(true)
              try {
                await onSave({
                  description: description.trim(),
                  url: url.trim(),
                  method: method.trim() || 'POST',
                  body: body || null,
                  secret: secret || null,
                  headers: headers.filter((h) => h.name.trim() !== ''),
                })
              } catch {
                // surfaced by parent
              } finally {
                setSaving(false)
              }
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
