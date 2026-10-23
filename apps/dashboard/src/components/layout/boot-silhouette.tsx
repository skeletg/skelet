import { useState, type CSSProperties } from 'react'
import { readBootPaint, type BootPaint } from '@/lib/boot-paint'

function boxStyle(b: BootPaint['sheet']): CSSProperties {
  return {
    position: 'fixed',
    left: b.left,
    top: b.top,
    right: b.right,
    bottom: b.bottom,
    borderRadius: b.radius,
    background: b.color,
    boxShadow: b.shadow,
  }
}

// React has already cleared index.html's painted outline while the shell's chunk loads, so this redraws it
export function BootSilhouette() {
  const [paint] = useState(readBootPaint)
  if (!paint) return null
  return (
    <div aria-hidden style={{ position: 'fixed', inset: 0, background: paint.page }}>
      {paint.rail && <div style={boxStyle(paint.rail)} />}
      <div style={boxStyle(paint.sheet)} />
    </div>
  )
}
