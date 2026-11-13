import { MinusIcon, PlusIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ZOOM_MAX, ZOOM_MIN, ZOOM_STEP } from './zoom'

function Hairline() {
  return <span aria-hidden className="h-3 w-px bg-border" />
}

// What a writer glances at, floating under the page: how long it is, its language, whether it is saved, how large it shows
export function PostCapsule({
  words,
  readEstimate,
  onUseEstimate,
  locale,
  unsaved,
  zoom,
  onZoom,
}: {
  words: number
  readEstimate: number
  /** Given while the estimate differs from the post's reading time and may be put there */
  onUseEstimate?: () => void
  locale: string
  unsaved: number | null
  zoom: number
  onZoom: (next: number) => void
}) {
  return (
    <div className="pointer-events-none sticky bottom-4 z-10 mb-4 flex justify-center px-4">
      <div className="pointer-events-auto flex max-w-full flex-wrap items-center justify-center gap-x-3 gap-y-1 rounded-full border border-foreground/10 bg-background/90 px-4 py-1.5 text-xs text-muted-foreground shadow-[0_1px_2px_rgb(0_0_0/0.06),0_10px_30px_-12px_rgb(0_0_0/0.25)] backdrop-blur-md">
        <span>
          <span className="tabular-nums font-medium text-foreground">{words.toLocaleString()}</span> {words === 1 ? 'word' : 'words'}
        </span>
        {words > 0 ? (
          <>
            <Hairline />
            <span className="flex items-center gap-2">
              {readEstimate} min read
              {onUseEstimate ? (
                <button type="button" className="font-medium text-foreground underline underline-offset-2 hover:no-underline" onClick={onUseEstimate}>
                  Use as reading time
                </button>
              ) : null}
            </span>
          </>
        ) : null}
        <Hairline />
        <span title="The language being written">{locale ? locale.toUpperCase() : 'No language'}</span>
        <Hairline />
        <span className={cn('flex items-center gap-1.5', unsaved != null && 'text-foreground')}>
          <span aria-hidden className={cn('size-1.5 rounded-full', unsaved != null ? 'bg-warning' : 'bg-success')} />
          {unsaved != null ? `${unsaved} unsaved ${unsaved === 1 ? 'change' : 'changes'}` : 'Saved'}
        </span>
        <Hairline />
        <span className="flex items-center gap-1">
          <button type="button" aria-label="Zoom out" disabled={zoom <= ZOOM_MIN} onClick={() => onZoom(zoom - ZOOM_STEP)} className="grid size-5 place-items-center rounded-full hover:bg-muted hover:text-foreground disabled:opacity-40">
            <MinusIcon className="size-3.5" />
          </button>
          <button type="button" aria-label="Zoom" title="Back to 100%" onClick={() => onZoom(100)} className="w-10 rounded-full text-center tabular-nums hover:text-foreground">
            {zoom}%
          </button>
          <button type="button" aria-label="Zoom in" disabled={zoom >= ZOOM_MAX} onClick={() => onZoom(zoom + ZOOM_STEP)} className="grid size-5 place-items-center rounded-full hover:bg-muted hover:text-foreground disabled:opacity-40">
            <PlusIcon className="size-3.5" />
          </button>
        </span>
      </div>
    </div>
  )
}
