import { useState, type MouseEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckIcon, CopyIcon, ExternalLinkIcon, FileTextIcon, SparklesIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  GenerateMediaAltTextDocument,
  MediaAiStatusDocument,
  SetMediaFocalPointDocument,
  UpdateMediaAssetDocument,
  type MediaAssetFieldsFragment as MediaAsset,
} from '../MediaLibrary.gql'

// The focused asset in full - its picture, what it is, and the words that travel with it. Edits save to the library,
// so every place using the asset gets them
export function AssetPanel({ asset, canManage, onChanged }: { asset: MediaAsset; canManage: boolean; onChanged: (a: MediaAsset) => void }) {
  const [draft, setDraft] = useState({ title: asset.title, altText: asset.altText, caption: asset.caption })
  const [shownFor, setShownFor] = useState(asset.id)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  // A different asset brings its own words - adjusted during render, not in an effect
  if (shownFor !== asset.id) {
    setShownFor(asset.id)
    setDraft({ title: asset.title, altText: asset.altText, caption: asset.caption })
    setError(null)
  }
  const dirty = draft.title !== asset.title || draft.altText !== asset.altText || draft.caption !== asset.caption

  const [update, { loading: saving }] = useMutation(UpdateMediaAssetDocument)
  const [setFocal] = useMutation(SetMediaFocalPointDocument)
  const ai = useQuery(MediaAiStatusDocument).data?.aiStatus.enabled ?? false
  const [generate, { loading: generating }] = useMutation(GenerateMediaAltTextDocument)
  const isImage = asset.mimeGroup === 'IMAGE'

  const run = async (fn: () => Promise<MediaAsset | undefined | null>) => {
    setError(null)
    try {
      const next = await fn()
      if (next) onChanged(next)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'That didn’t save.')
    }
  }
  const pickFocal = (e: MouseEvent<HTMLImageElement>) => {
    const r = e.currentTarget.getBoundingClientRect()
    const x = Number(Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)).toFixed(4))
    const y = Number(Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)).toFixed(4))
    void run(async () => (await setFocal({ variables: { assetId: asset.id, x, y } })).data?.setMediaFocalPoint)
  }

  return (
    <div className="grid content-start gap-4">
      <div className="grid place-items-center overflow-hidden rounded-md border bg-muted/40 p-2">
        {isImage ? (
          <div className="relative">
            <img
              src={asset.url}
              alt={asset.altText || asset.filename}
              onClick={canManage ? pickFocal : undefined}
              className={cn('max-h-64 max-w-full rounded', canManage && 'cursor-crosshair')}
            />
            {asset.focalX != null && asset.focalY != null ? (
              <span
                aria-hidden
                className="pointer-events-none absolute size-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow ring-1 ring-black/40"
                style={{ left: `${asset.focalX * 100}%`, top: `${asset.focalY * 100}%` }}
              />
            ) : null}
          </div>
        ) : (
          <FileTextIcon className="my-10 size-12 text-muted-foreground" />
        )}
      </div>
      {isImage && canManage ? (
        <p className="-mt-2 text-xs text-muted-foreground">
          Click the picture to set the point crops keep in frame.
          {asset.focalX != null ? (
            <>
              {' '}
              <button type="button" className="underline underline-offset-2" onClick={() => void run(async () => (await setFocal({ variables: { assetId: asset.id, x: null, y: null } })).data?.setMediaFocalPoint)}>
                Reset to centre
              </button>
            </>
          ) : null}
        </p>
      ) : null}

      <div className="grid gap-0.5">
        <p className="break-all text-sm font-medium">{asset.filename}</p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          <dt>Folder</dt>
          <dd className="truncate">{asset.directoryPath || 'Library'}</dd>
          <dt>Type</dt>
          <dd>{asset.mimeType}</dd>
          <dt>Size</dt>
          <dd className="tabular-nums">
            {formatBytes(asset.sizeBytes)}
            {asset.width && asset.height ? ` · ${asset.width}×${asset.height}` : ''}
          </dd>
          <dt>Added</dt>
          <dd>{new Date(asset.createdAt).toLocaleString()}</dd>
        </dl>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          type="button" size="sm" variant="outline"
          onClick={() => {
            void navigator.clipboard?.writeText(asset.url).then(() => {
              setCopied(true)
              setTimeout(() => setCopied(false), 1500)
            })
          }}
        >
          {copied ? <CheckIcon className="size-3.5" /> : <CopyIcon className="size-3.5" />}
          {copied ? 'Copied' : 'Copy link'}
        </Button>
        <Button asChild size="sm" variant="ghost">
          <a href={asset.url} target="_blank" rel="noreferrer">
            <ExternalLinkIcon className="size-3.5" />
            Open
          </a>
        </Button>
        <Button asChild size="sm" variant="ghost">
          <Link to={`/media?path=${encodeURIComponent(asset.directoryPath)}`}>In the library</Link>
        </Button>
      </div>

      <fieldset disabled={!canManage} className="grid gap-3 border-t pt-4">
        <div className="grid gap-1.5">
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor="picker-alt">Alt text</Label>
            {ai && isImage && canManage ? (
              <Button
                type="button" size="sm" variant="ghost" className="h-6 px-2 text-xs" disabled={generating}
                onClick={() =>
                  void run(async () => {
                    const text = (await generate({ variables: { assetId: asset.id } })).data?.generateMediaAltText
                    if (text) setDraft((d) => ({ ...d, altText: text }))
                    return null
                  })
                }
              >
                <SparklesIcon className="size-3" />
                {generating ? 'Writing…' : 'Write with AI'}
              </Button>
            ) : null}
          </div>
          <Textarea id="picker-alt" rows={2} value={draft.altText} placeholder="What the picture shows, for screen readers and search" onChange={(e) => setDraft({ ...draft, altText: e.target.value })} />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="picker-title">Title</Label>
          <Input id="picker-title" value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="picker-caption">Caption</Label>
          <Textarea id="picker-caption" rows={2} value={draft.caption} onChange={(e) => setDraft({ ...draft, caption: e.target.value })} />
        </div>
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
        {canManage ? (
          <div className="flex items-center gap-2">
            <Button
              type="button" size="sm" disabled={!dirty || saving}
              onClick={() => void run(async () => (await update({ variables: { input: { id: asset.id, ...draft } } })).data?.updateMediaAsset)}
            >
              {saving ? 'Saving…' : 'Save details'}
            </Button>
            {dirty ? (
              <Button type="button" size="sm" variant="ghost" onClick={() => setDraft({ title: asset.title, altText: asset.altText, caption: asset.caption })}>
                Undo
              </Button>
            ) : null}
          </div>
        ) : null}
      </fieldset>
    </div>
  )
}
