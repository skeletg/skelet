import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  CmsPageDocument,
  SetCmsPageExperimentDocument,
  UpsertCmsPageVariantContentDocument,
  type CmsPageRowFragment,
} from '../Cms.gql'
import { ExperimentsDocument } from '@/features/prism/Prism.signals.gql'
import { PageBlockCanvas } from '../PageBlockCanvas'
import { parseDocument, serializeDocument, type CmsBlock } from '../page-blocks'

// CmsPageExperimentSection binds the page to a Prism experiment and authors a block layout per variation (control = the
// page's main content) - Visitors are server-assigned at render; conversions ride the experiment's goal event
export function CmsPageExperimentSection({
  page,
  locale,
  channelId,
  canManage,
  refetchVariables,
}: {
  page: CmsPageRowFragment
  locale: string
  channelId: string | null
  canManage: boolean
  refetchVariables: { id: string }
}) {
  const refetch = [{ query: CmsPageDocument, variables: refetchVariables }]
  const { data } = useQuery(ExperimentsDocument, {
    variables: { channelId: channelId ?? '' },
    skip: !channelId,
  })
  const experiments = data?.experiments ?? []
  const [setExperiment, { loading: binding }] = useMutation(SetCmsPageExperimentDocument, { refetchQueries: refetch })
  const [pick, setPick] = useState('')
  const bound = experiments.find((e) => e.key === page.experimentKey)

  if (!page.experimentKey) {
    return (
      <div className="grid max-w-xl gap-3">
        {!channelId ? (
          <p className="text-sm text-muted-foreground">Pick a channel in the header to bind a Prism experiment.</p>
        ) : experiments.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No experiments on this channel yet - create one under <em>Signals → Prism</em>.
          </p>
        ) : (
          <div className="flex flex-wrap items-end gap-2">
            <Select value={pick} onValueChange={setPick}>
              <SelectTrigger className="w-72">
                <SelectValue placeholder="Select an experiment…" />
              </SelectTrigger>
              <SelectContent>
                {experiments.map((e) => (
                  <SelectItem key={e.id} value={e.key}>
                    {e.name} ({e.status})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              disabled={!canManage || !pick || binding}
              onClick={() => setExperiment({ variables: { pageId: page.id, experimentKey: pick } })}
            >
              Bind
            </Button>
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="secondary">{bound ? bound.name : page.experimentKey}</Badge>
        <code className="text-xs text-muted-foreground">{page.experimentKey}</code>
        {bound && <Badge variant="outline" className="text-xs">{bound.status}</Badge>}
        {canManage && (
          <Button
            variant="ghost"
            size="sm"
            disabled={binding}
            onClick={() => setExperiment({ variables: { pageId: page.id, experimentKey: '' } })}
          >
            Unbind
          </Button>
        )}
      </div>

      {!bound ? (
        <Alert>
          <AlertTitle>Experiment not found on this channel</AlertTitle>
          <AlertDescription>
            The bound key “{page.experimentKey}” has no matching experiment on the selected channel. Create or select it
            in Prism, or unbind.
          </AlertDescription>
        </Alert>
      ) : !locale ? (
        <p className="text-sm text-muted-foreground">Pick a locale in the header to edit variation layouts.</p>
      ) : (
        <div className="grid gap-4">
          <p className="text-xs text-muted-foreground">
            Control = the page&apos;s main content (edited below). Author a layout for each other variation. Preview one
            on the storefront with <code>?prism={page.experimentKey}:&lt;variation&gt;</code>.
          </p>
          {bound.variations.map((v, i) =>
            i === 0 ? (
              <div key={v.key} className="rounded-md border bg-muted/30 px-3 py-2 text-sm">
                <span className="font-medium">{v.name}</span>
                <Badge variant="outline" className="ml-2 text-[10px]">control</Badge>
                <span className="ml-2 text-xs text-muted-foreground">uses the page&apos;s main content</span>
              </div>
            ) : (
              <VariantBodyEditor
                key={`${v.key}:${locale}`}
                pageId={page.id}
                locale={locale}
                variationKey={v.key}
                variationName={v.name}
                initialBody={page.variants.find((pv) => pv.variantKey === v.key)?.body}
                canManage={canManage}
                refetch={refetch}
              />
            ),
          )}
        </div>
      )}
    </div>
  )
}

// VariantBodyEditor authors one A/B variation's block layout (saved to the
// page's variant body for the active locale)
function VariantBodyEditor({
  pageId,
  locale,
  variationKey,
  variationName,
  initialBody,
  canManage,
  refetch,
}: {
  pageId: string
  locale: string
  variationKey: string
  variationName: string
  initialBody: unknown
  canManage: boolean
  refetch: { query: typeof CmsPageDocument; variables: { id: string } }[]
}) {
  const [upsert, { loading: saving }] = useMutation(UpsertCmsPageVariantContentDocument, { refetchQueries: refetch })
  const [blocks, setBlocks] = useState<CmsBlock[]>(() => parseDocument(initialBody)?.blocks ?? [])
  const [ok, setOk] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function save() {
    setError(null)
    setOk(false)
    try {
      const body = blocks.length > 0 ? serializeDocument(blocks) : null
      await upsert({ variables: { pageId, locale, variantKey: variationKey, body } })
      setOk(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
    }
  }

  return (
    <div className="grid gap-2 rounded-md border p-3">
      <Label className="flex items-center gap-2 text-sm font-medium">
        {variationName}
        <code className="text-xs font-normal text-muted-foreground">{variationKey}</code>
      </Label>
      <PageBlockCanvas blocks={blocks} onChange={setBlocks} disabled={!canManage} />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          <Button size="sm" onClick={save} disabled={saving}>
            {saving ? 'Saving…' : `Save ${variationName} layout`}
          </Button>
          {ok && <span className="text-xs text-emerald-600">Saved.</span>}
        </div>
      )}
    </div>
  )
}
