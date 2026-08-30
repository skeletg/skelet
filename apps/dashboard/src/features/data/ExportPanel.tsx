import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { DownloadIcon, Loader2Icon } from 'lucide-react'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useConfigScope } from '@/lib/config-scope'
import { StartDataExportDocument, type DataResourceFieldsFragment } from './Data.gql'
import { OptionsForm } from './OptionsForm'
import { missingRequired } from './jobs'

export function ExportPanel({
  resource,
  formats,
  onQueued,
}: {
  resource: DataResourceFieldsFragment
  formats: Array<{ format: string; label: string; extension: string }>
  onQueued: (jobId: string) => void
}) {
  const { effectiveChannelId, effectiveLocale } = useConfigScope()
  const [format, setFormat] = useState('csv')
  const [values, setValues] = useState<Record<string, string>>({})
  const [error, setError] = useState<string | null>(null)
  const [start, { loading }] = useMutation(StartDataExportDocument)

  const blocking = missingRequired(resource.options, values, 'export')

  async function run() {
    setError(null)
    try {
      const res = await start({
        variables: {
          input: {
            resource: resource.slug,
            format,
            channelId: effectiveChannelId ?? null,
            locale: effectiveLocale || null,
            options: Object.entries(values)
              .filter(([, v]) => v !== '')
              .map(([name, value]) => ({ name, value })),
          },
        },
      })
      const id = res.data?.startDataExport.id
      if (id) {
        toast.success(`Exporting ${resource.label.toLowerCase()} - it appears below when ready`)
        onQueued(id)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not queue the export')
    }
  }

  return (
    <div className="grid gap-5">
      <p className="text-sm text-muted-foreground">
        {resource.description}
        {effectiveChannelId
          ? ' Scoped to the channel selected in the header.'
          : ' Covering every channel you can see.'}
      </p>

      <div className="grid gap-1.5 sm:max-w-xs">
        <Label htmlFor="export-format">Format</Label>
        <Select value={format} onValueChange={setFormat}>
          <SelectTrigger id="export-format">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {formats.map((f) => (
              <SelectItem key={f.format} value={f.format}>
                {f.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <OptionsForm
        options={resource.options}
        values={values}
        direction="export"
        onChange={(name, value) => setValues((prev) => ({ ...prev, [name]: value }))}
      />

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>Export failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex items-center gap-3">
        <Button onClick={run} disabled={loading || blocking.length > 0}>
          {loading ? (
            <Loader2Icon className="mr-2 size-4 animate-spin" />
          ) : (
            <DownloadIcon className="mr-2 size-4" />
          )}
          Export {resource.label.toLowerCase()}
        </Button>
        {blocking.length > 0 ? (
          <p className="text-sm text-muted-foreground">Choose {blocking.join(' and ')} first</p>
        ) : (
          <p className="text-sm text-muted-foreground">
            The file is built in the background - it appears below when it is ready.
          </p>
        )}
      </div>
    </div>
  )
}
