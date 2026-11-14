import { useEffect, useState, type MouseEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckIcon, CopyIcon, DownloadIcon, ExternalLinkIcon, FileTextIcon, LockIcon, SparklesIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { mediaFileUrl } from '@/lib/api-endpoint'
import { useDownload } from '@/lib/authed-download'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  GenerateMediaAltTextDocument,
  MediaAiStatusDocument,
  SetMediaFocalPointDocument,
  UpdateMediaAssetDocument,
  type MediaAssetFieldsFragment as MediaAsset,
} from '../MediaLibrary.gql'
import { usePrivatePicture } from '../private-image'

const PREVIEW_WIDTH = 960

// The focused asset in full - its picture, what it is, and the words that travel with it. Edits save to the library,
// so every place using the asset gets them
export function AssetPanel({
  asset,
  canManage,
  onChanged,
  notes = false,
  libraryLink = true,
  onDirty,
}: {
  asset: MediaAsset
  canManage: boolean
  onChanged: (a: MediaAsset) => void
  /** Also edit the notes kept with the asset - the library does, a picker doesn't need them */
  notes?: boolean
  libraryLink?: boolean
  /** Told whenever typed words are waiting to be saved, so whoever holds the panel can ask before dropping them */
  onDirty?: (dirty: boolean) => void
}) {
  const words = (a: MediaAsset) => ({ title: a.title, altText: a.altText, caption: a.caption, ...(notes ? { notes: a.notes } : {}) })
  const [draft, setDraft] = useState(() => words(asset))
  const [shownFor, setShownFor] = useState(asset.id)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  // A different asset brings its own words - adjusted during render, not in an effect
  if (shownFor !== asset.id) {
    setShownFor(asset.id)
    setDraft(words(asset))
    setError(null)
  }
  const dirty = draft.title !== asset.title || draft.altText !== asset.altText || draft.caption !== asset.caption || (notes && draft.notes !== asset.notes)
  useEffect(() => {
    onDirty?.(dirty)
    return () => onDirty?.(false)
  }, [dirty, onDirty])

  const [update, { loading: saving }] = useMutation(UpdateMediaAssetDocument)
  const [setFocal] = useMutation(SetMediaFocalPointDocument)
  const ai = useQuery(MediaAiStatusDocument).data?.aiStatus.enabled ?? false
  const [generate, { loading: generating }] = useMutation(GenerateMediaAltTextDocument)
  const isImage = asset.mimeGroup === 'IMAGE'
  const isPrivate = asset.visibility === 'PRIVATE'
  const picture = usePrivatePicture(asset, PREVIEW_WIDTH)
  const src = isPrivate ? picture.url : asset.url
  const [broken, setBroken] = useState<string | null>(null)
  const download = useDownload((e) => setError(e instanceof Error ? e.message : "The file couldn't be downloaded."))

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
        {isImage && src && broken !== src ? (
          <div className="relative">
            <img
              src={src}
              alt={asset.altText || asset.filename}
              onClick={canManage ? pickFocal : undefined}
              onError={() => setBroken(src)}
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
        ) : isPrivate && picture.shown && !picture.failed && !picture.url ? (
          <p className="my-10 text-xs text-muted-foreground">Loading the picture…</p>
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

      {isPrivate ? (
        <p className="flex items-start gap-2 rounded-md border border-info/40 bg-info-soft px-3 py-2 text-xs">
          <LockIcon className="mt-0.5 size-3.5 shrink-0 text-info" />
          Private. It has no address, so it can't be linked to or put on the storefront - only admins, and customers it is shared with, can open it.
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {isPrivate ? (
          <Button type="button" size="sm" variant="outline" disabled={download.busy !== null} onClick={() => download.start(mediaFileUrl(asset.id), asset.filename)}>
            <DownloadIcon className="size-3.5" />
            {download.busy ? 'Downloading…' : 'Download'}
          </Button>
        ) : (
          <>
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
          </>
        )}
        {libraryLink ? (
          <Button asChild size="sm" variant="ghost">
            <Link to={`/media?path=${encodeURIComponent(asset.directoryPath)}`}>In the library</Link>
          </Button>
        ) : null}
      </div>

      <fieldset disabled={!canManage} className="grid gap-3 border-t pt-4">
        <div className="grid gap-1.5">
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor="picker-alt">Alt text</Label>
            {ai && isImage && canManage && !isPrivate ? (
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
        {notes ? (
          <div className="grid gap-1.5">
            <Label htmlFor="picker-notes">Notes</Label>
            <Textarea id="picker-notes" rows={2} value={draft.notes ?? ''} placeholder="For your team - never shown on the storefront" onChange={(e) => setDraft({ ...draft, notes: e.target.value })} />
          </div>
        ) : null}
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
              <Button type="button" size="sm" variant="ghost" onClick={() => setDraft(words(asset))}>
                Undo
              </Button>
            ) : null}
          </div>
        ) : null}
      </fieldset>
    </div>
  )
}
