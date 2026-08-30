import { useRef, useState } from 'react'
import { FileUpIcon, Loader2Icon, TableIcon, UploadIcon } from 'lucide-react'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useConfigScope } from '@/lib/config-scope'
import { downloadTemplate, uploadImport } from './api'
import { ColumnReference } from './ColumnReference'
import type { DataResourceFieldsFragment } from './Data.gql'
import { OptionsForm } from './OptionsForm'
import { missingRequired } from './jobs'

export function ImportPanel({
  resource,
  formats,
  onQueued,
}: {
  resource: DataResourceFieldsFragment
  formats: Array<{ format: string; label: string; extension: string }>
  onQueued: (jobId: string) => void
}) {
  const { effectiveChannelId, effectiveLocale } = useConfigScope()
  const [file, setFile] = useState<File | null>(null)
  const [dryRun, setDryRun] = useState(true)
  const [values, setValues] = useState<Record<string, string>>({})
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const accept = formats.map((f) => `.${f.extension}`).join(',')
  const blocking = missingRequired(resource.options, values, 'import')
  const csv = formats.find((f) => f.format === 'csv') ?? formats[0]

  async function run() {
    if (!file) return
    setBusy(true)
    setError(null)
    try {
      const { id } = await uploadImport({
        resource: resource.slug,
        file,
        channelId: effectiveChannelId,
        locale: effectiveLocale || undefined,
        dryRun,
        options: values,
      })
      setFile(null)
      if (inputRef.current) inputRef.current.value = ''
      toast.success(dryRun ? 'Validating the file - the result appears below' : `Importing ${resource.label.toLowerCase()}`)
      onQueued(id)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not start the import')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-sm text-muted-foreground">
          Rows are upserted on{' '}
          {resource.fields
            .filter((f) => f.identifier)
            .map((f) => f.name)
            .join(' + ') || 'their key columns'}
          . A row that fails is reported on its own line and the rest still land.
        </p>
        {csv ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => downloadTemplate(resource.slug, csv.format, csv.extension)}
          >
            <TableIcon className="mr-2 size-4" />
            Download template
          </Button>
        ) : null}
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="import-file">File</Label>
        <Input
          id="import-file"
          ref={inputRef}
          type="file"
          accept={accept}
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
        />
        <p className="text-xs text-muted-foreground">
          Accepts {formats.map((f) => f.label).join(' or ')}. The format is taken from the file
          extension.
        </p>
      </div>

      <OptionsForm
        options={resource.options}
        values={values}
        direction="import"
        onChange={(name, value) => setValues((prev) => ({ ...prev, [name]: value }))}
      />

      <div className="flex items-start gap-3 rounded-md border p-3">
        <Switch id="import-dry-run" checked={dryRun} onCheckedChange={setDryRun} />
        <div className="grid gap-0.5">
          <Label htmlFor="import-dry-run" className="cursor-pointer">
            Validate only
          </Label>
          <p className="text-xs text-muted-foreground">
            Runs every row through the same checks and reports what would happen, without writing
            anything. Worth doing once on any file you did not export from here.
          </p>
        </div>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>Import failed to start</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex items-center gap-3">
        <Button onClick={run} disabled={!file || busy || blocking.length > 0}>
          {busy ? (
            <Loader2Icon className="mr-2 size-4 animate-spin" />
          ) : dryRun ? (
            <FileUpIcon className="mr-2 size-4" />
          ) : (
            <UploadIcon className="mr-2 size-4" />
          )}
          {dryRun ? 'Validate file' : `Import ${resource.label.toLowerCase()}`}
        </Button>
        {blocking.length > 0 ? (
          <p className="text-sm text-muted-foreground">Choose {blocking.join(' and ')} first</p>
        ) : null}
      </div>

      <div className="grid gap-2">
        <h4 className="text-sm font-medium">Columns</h4>
        <ColumnReference resource={resource} />
      </div>
    </div>
  )
}
