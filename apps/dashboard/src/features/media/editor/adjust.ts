export type Adjust = { brightness: number; contrast: number; saturation: number }

export const ADJUST_MAX = 50
export const NEUTRAL: Adjust = { brightness: 0, contrast: 0, saturation: 0 }

const factor = (value: number) => 1 + value / 100

export const isNeutral = (a: Adjust) => a.brightness === 0 && a.contrast === 0 && a.saturation === 0

export const cssFilter = (a: Adjust) => (isNeutral(a) ? 'none' : `brightness(${factor(a.brightness)}) contrast(${factor(a.contrast)}) saturate(${factor(a.saturation)})`)

const clamp = (v: number) => (v < 0 ? 0 : v > 255 ? 255 : v)

// The CSS filters' own arithmetic, so what is saved is what the stage showed - by hand, since not every browser's canvas takes a filter
export function applyAdjust(pixels: Uint8ClampedArray, a: Adjust) {
  if (isNeutral(a)) return
  const b = factor(a.brightness)
  const c = factor(a.contrast)
  const s = factor(a.saturation)
  for (let i = 0; i < pixels.length; i += 4) {
    const r = clamp((clamp(pixels[i]! * b) - 127.5) * c + 127.5)
    const g = clamp((clamp(pixels[i + 1]! * b) - 127.5) * c + 127.5)
    const bl = clamp((clamp(pixels[i + 2]! * b) - 127.5) * c + 127.5)
    pixels[i] = (0.213 + 0.787 * s) * r + (0.715 - 0.715 * s) * g + (0.072 - 0.072 * s) * bl
    pixels[i + 1] = (0.213 - 0.213 * s) * r + (0.715 + 0.285 * s) * g + (0.072 - 0.072 * s) * bl
    pixels[i + 2] = (0.213 - 0.213 * s) * r + (0.715 - 0.715 * s) * g + (0.072 + 0.928 * s) * bl
  }
}
