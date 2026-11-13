import { useState } from 'react'

const ZOOM_KEY = 'skelet:blog-editor:zoom'
export const ZOOM_MIN = 70
export const ZOOM_MAX = 150
export const ZOOM_STEP = 10

function readZoom(): number {
  try {
    const z = Number(window.localStorage.getItem(ZOOM_KEY))
    return z >= ZOOM_MIN && z <= ZOOM_MAX ? z : 100
  } catch {
    return 100
  }
}

export function usePageZoom(): [number, (next: number) => void] {
  const [zoom, setZoom] = useState(readZoom)
  return [
    zoom,
    (next) => {
      const z = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, next))
      setZoom(z)
      try {
        window.localStorage.setItem(ZOOM_KEY, String(z))
      } catch {
        // the zoom just won't be remembered
      }
    },
  ]
}
