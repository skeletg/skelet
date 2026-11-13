import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { useMutation } from '@apollo/client/react'
import { FlipHorizontal2Icon, FlipVertical2Icon, LoaderIcon, RotateCcwIcon, RotateCwIcon, XIcon } from 'lucide-react'
import { Cropper, type CropperRef, type CropperState } from 'react-advanced-cropper'
import 'react-advanced-cropper/dist/style.css'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { UpdateMediaAssetDocument, type MediaAssetFieldsFragment as MediaAsset } from '../MediaLibrary.gql'
import { useMediaUpload } from '../useImageUpload'
import { ADJUST_MAX, NEUTRAL, cssFilter, isNeutral, type Adjust } from './adjust'
import { editedName, renderEdited } from './output'
import { useOriginal } from './useOriginal'

type Ratio = { id: string; label: string; value?: number }
const RATIOS: Ratio[] = [
  { id: 'free', label: 'Free' },
  { id: 'original', label: 'Original' },
  { id: 'square', label: 'Square', value: 1 },
  { id: '4:3', label: '4:3', value: 4 / 3 },
  { id: '3:2', label: '3:2', value: 3 / 2 },
  { id: '16:9', label: '16:9', value: 16 / 9 },
]
const SLIDERS: { key: keyof Adjust; label: string }[] = [
  { key: 'brightness', label: 'Brightness' },
  { key: 'contrast', label: 'Contrast' },
  { key: 'saturation', label: 'Saturation' },
]
const QUARTER = 90
const sideways = (degrees: number) => Math.round(degrees / QUARTER) % 2 !== 0
const wholePicture = ({ imageSize, transforms }: CropperState) => (sideways(transforms.rotate) ? { width: imageSize.height, height: imageSize.width } : imageSize)

// Crop, turn, mirror, light and colour. The original is never touched: the edit is a new file beside it, since a file's address is kept for good
export default function PhotoEditor({ asset, onClose, onSaved }: { asset: MediaAsset; onClose: () => void; onSaved: (assetId: string) => void }) {
  const original = useOriginal(asset.id)
  const cropper = useRef<CropperRef>(null)
  const upload = useMediaUpload(asset.directoryPath)
  const [updateAsset] = useMutation(UpdateMediaAssetDocument)

  const [ratio, setRatio] = useState('free')
  const [upright, setUpright] = useState(false)
  const [turns, setTurns] = useState(0)
  const [mirrored, setMirrored] = useState({ across: false, down: false })
  const [adjust, setAdjust] = useState<Adjust>(NEUTRAL)
  const [frame, setFrame] = useState<{ width: number; height: number; whole: boolean } | null>(null)
  const [stageError, setStageError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [asking, setAsking] = useState(false)

  const edited = turns !== 0 || mirrored.across || mirrored.down || !isNeutral(adjust) || (frame !== null && !frame.whole)
  const chosen = RATIOS.find((r) => r.id === ratio)
  const natural = asset.width && asset.height ? asset.width / asset.height : undefined
  const aspect = ratio === 'original' ? (natural && turns % 2 === 1 ? 1 / natural : natural) : chosen?.value ? (upright ? 1 / chosen.value : chosen.value) : undefined

  // A shape takes the largest piece of the picture that has it - left alone the library only narrows the frame, and doesn't report that it did
  useEffect(() => {
    if (ratio !== 'free') cropper.current?.setCoordinates(wholePicture)
  }, [ratio, aspect])

  function measure(c: CropperRef) {
    const box = c.getCoordinates()
    const state = c.getState()
    if (!box || !state) return
    const full = wholePicture(state)
    const width = Math.round(box.width)
    const height = Math.round(box.height)
    setFrame((was) => (was?.width === width && was.height === height ? was : { width, height, whole: width >= full.width && height >= full.height }))
  }

  function turn(by: number) {
    const c = cropper.current
    if (!c) return
    c.rotateImage(by * QUARTER)
    // a frame holding the whole picture turns with it, rather than being cut down to what still fits
    if (frame?.whole) c.setCoordinates(wholePicture)
    setTurns((t) => (t + by + 4) % 4)
  }
  function mirror(across: boolean) {
    cropper.current?.flipImage(across, !across)
    setMirrored((m) => (across ? { ...m, across: !m.across } : { ...m, down: !m.down }))
  }
  function startOver() {
    void cropper.current?.reset()
    setRatio('free')
    setUpright(false)
    setTurns(0)
    setMirrored({ across: false, down: false })
    setAdjust(NEUTRAL)
    setError(null)
  }
  const close = () => (edited && !saving ? setAsking(true) : onClose())

  async function save() {
    const canvas = cropper.current?.getCanvas()
    if (!canvas) return setError("The picture isn't ready yet.")
    setSaving(true)
    setError(null)
    try {
      const blob = await renderEdited(canvas, adjust, asset.mimeType)
      const copy = await upload(new File([blob], editedName(asset.filename), { type: blob.type }))
      if (asset.title || asset.altText || asset.caption) {
        await updateAsset({ variables: { input: { id: copy.id, title: asset.title, altText: asset.altText, caption: asset.caption } } }).catch(() => {})
      }
      onSaved(copy.id)
    } catch (e) {
      setError(e instanceof Error ? e.message : "The copy couldn't be saved.")
      setSaving(false)
    }
  }

  const problem = original.error ?? stageError
  return (
    <Dialog open onOpenChange={(open) => !open && close()}>
      <DialogContent showCloseButton={false} aria-describedby={undefined} className="flex h-[min(46rem,calc(100vh-2rem))] flex-col gap-0 overflow-hidden p-0 sm:max-w-[min(72rem,calc(100%-2rem))]">
        <header className="flex h-[49px] shrink-0 items-center justify-between gap-3 border-b border-foreground/10 pr-2 pl-4">
          <DialogTitle className="min-w-0 truncate text-sm font-medium tracking-tight">Edit {asset.filename}</DialogTitle>
          <div className="flex shrink-0 items-center gap-3">
            {frame ? (
              <span className="text-xs text-muted-foreground tabular-nums" aria-label="Size of the edited picture">
                {frame.width} × {frame.height} px
              </span>
            ) : null}
            <Button type="button" variant="ghost" size="icon" className="size-8 rounded-full text-muted-foreground" aria-label="Close the editor" onClick={close}>
              <XIcon className="size-4" />
            </Button>
          </div>
        </header>

        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          <div className="relative min-h-0 min-w-0 flex-1 bg-black" style={{ '--photo-filter': cssFilter(adjust) } as CSSProperties}>
            {problem ? (
              <p role="alert" className="absolute inset-0 grid place-items-center p-8 text-center text-sm text-white/80">
                {problem}
              </p>
            ) : original.url ? (
              <Cropper
                ref={cropper}
                src={original.url}
                className="size-full"
                backgroundClassName="[filter:var(--photo-filter)]"
                stencilProps={{ aspectRatio: aspect, grid: true }}
                defaultSize={wholePicture}
                onReady={measure}
                onUpdate={measure}
                onError={() => setStageError("This picture couldn't be shown for editing.")}
              />
            ) : (
              <span className="absolute inset-0 grid place-items-center text-white/70">
                <LoaderIcon className="size-6 animate-spin" aria-label="Opening the picture" />
              </span>
            )}
          </div>

          <aside aria-label="Tools" className="flex max-h-[45%] w-full shrink-0 flex-col border-t border-foreground/10 md:max-h-none md:w-72 md:border-t-0 md:border-l">
            <div className="grid flex-1 content-start gap-6 overflow-y-auto p-4">
              <section className="grid gap-2" aria-label="Crop">
                <h3 className="text-xs font-medium text-muted-foreground">Crop to a shape</h3>
                <div className="flex flex-wrap gap-1">
                  {RATIOS.map((r) => (
                    <button
                      key={r.id}
                      type="button"
                      aria-pressed={ratio === r.id}
                      onClick={() => setRatio(r.id)}
                      className={cn(
                        'rounded-full px-2.5 py-1 text-xs outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                        ratio === r.id ? 'bg-foreground text-background' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                      )}
                    >
                      {r.label}
                    </button>
                  ))}
                </div>
                {chosen?.value && chosen.value !== 1 ? (
                  <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <input type="checkbox" className="accent-foreground" checked={upright} onChange={(e) => setUpright(e.target.checked)} />
                    Upright, taller than wide
                  </label>
                ) : null}
                <p className="text-xs text-muted-foreground">Drag the corners on the picture to choose what stays.</p>
              </section>

              <section className="grid gap-2" aria-label="Turn">
                <h3 className="text-xs font-medium text-muted-foreground">Turn and mirror</h3>
                <div className="grid grid-cols-2 gap-1.5">
                  <Tool label="Turn left" onClick={() => turn(-1)} icon={RotateCcwIcon} />
                  <Tool label="Turn right" onClick={() => turn(1)} icon={RotateCwIcon} />
                  <Tool label="Mirror" pressed={mirrored.across} onClick={() => mirror(true)} icon={FlipHorizontal2Icon} />
                  <Tool label="Upside down" pressed={mirrored.down} onClick={() => mirror(false)} icon={FlipVertical2Icon} />
                </div>
              </section>

              <section className="grid gap-3" aria-label="Light and colour">
                <h3 className="text-xs font-medium text-muted-foreground">Light and colour</h3>
                {SLIDERS.map((s) => (
                  <div key={s.key} className="grid gap-1">
                    <div className="flex items-center justify-between text-xs">
                      <label htmlFor={`photo-${s.key}`}>{s.label}</label>
                      <span className="text-muted-foreground tabular-nums">{adjust[s.key] > 0 ? `+${adjust[s.key]}` : adjust[s.key]}</span>
                    </div>
                    <input
                      id={`photo-${s.key}`}
                      type="range"
                      min={-ADJUST_MAX}
                      max={ADJUST_MAX}
                      value={adjust[s.key]}
                      onChange={(e) => setAdjust({ ...adjust, [s.key]: Number(e.target.value) })}
                      onDoubleClick={() => setAdjust({ ...adjust, [s.key]: 0 })}
                      className="h-1 w-full cursor-pointer accent-foreground"
                    />
                  </div>
                ))}
              </section>
            </div>

            <footer className="grid gap-2 border-t border-foreground/10 p-4">
              {error ? (
                <p role="alert" className="text-xs text-destructive">
                  {error}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  The original stays as it is. The edit is saved beside it as <span className="font-medium text-foreground">{editedName(asset.filename)}</span>.
                </p>
              )}
              <div className="flex items-center justify-between gap-2">
                <Button type="button" variant="ghost" size="sm" disabled={!edited || saving} onClick={startOver}>
                  Start over
                </Button>
                <Button type="button" size="sm" disabled={!edited || saving || !!problem || !frame} onClick={() => void save()}>
                  {saving ? 'Saving…' : 'Save as a copy'}
                </Button>
              </div>
            </footer>
          </aside>
        </div>

        <ConfirmDialog
          open={asking}
          onOpenChange={setAsking}
          title="Leave without saving the edit?"
          description="The crop, turns and colour changes you made will be lost. The original isn't affected either way."
          confirmLabel="Leave"
          cancelLabel="Keep editing"
          destructive
          onConfirm={onClose}
        />
      </DialogContent>
    </Dialog>
  )
}

function Tool({ label, icon: Icon, pressed, onClick }: { label: string; icon: typeof XIcon; pressed?: boolean; onClick: () => void }) {
  return (
    <Button type="button" variant={pressed ? 'secondary' : 'outline'} size="sm" className="justify-start" aria-pressed={pressed} onClick={onClick}>
      <Icon className="size-4" />
      {label}
    </Button>
  )
}
