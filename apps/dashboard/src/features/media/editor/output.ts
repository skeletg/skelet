import { applyAdjust, isNeutral, type Adjust } from './adjust'

export const EDITABLE = ['image/jpeg', 'image/png', 'image/webp']
const QUALITY = 0.92
const TAG = '-edited'

export const isEditable = (a: { mimeType: string; status: string }) => a.status === 'READY' && EDITABLE.includes(a.mimeType)

// boot.jpg becomes boot-edited.jpg, and editing that one again doesn't stack the word
export function editedName(filename: string) {
  const dot = filename.lastIndexOf('.')
  const base = dot > 0 ? filename.slice(0, dot) : filename
  const ext = dot > 0 ? filename.slice(dot) : ''
  return `${base.endsWith(TAG) ? base : base + TAG}${ext}`
}

export function renderEdited(canvas: HTMLCanvasElement, adjust: Adjust, mimeType: string): Promise<Blob> {
  if (!isNeutral(adjust)) {
    const ctx = canvas.getContext('2d')
    if (!ctx) return Promise.reject(new Error("This browser couldn't work on the picture."))
    const image = ctx.getImageData(0, 0, canvas.width, canvas.height)
    applyAdjust(image.data, adjust)
    ctx.putImageData(image, 0, 0)
  }
  const type = EDITABLE.includes(mimeType) ? mimeType : 'image/png'
  return new Promise((resolve, reject) =>
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("The edited picture couldn't be written."))), type, type === 'image/png' ? undefined : QUALITY),
  )
}
