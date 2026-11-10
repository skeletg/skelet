import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { calm } from './useAssistant'

// The Agentic Commerce pages share one look: a stage at the top of each, the assistant's voice on it, and words that
// arrive the way a model writes them

export function AuroraStage({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('aurora-stage relative overflow-hidden rounded-md border', className)}>{children}</div>
}

const SIZES = { sm: 24, md: 40, lg: 72 } as const
const BOX = { sm: 'size-6', md: 'size-10', lg: 'size-18' } as const
const DETAIL = { sm: { layers: 3, sparks: 0, rings: false }, md: { layers: 4, sparks: 18, rings: true }, lg: { layers: 4, sparks: 34, rings: true } } as const
const SPREAD = 1.6 // the canvas reaches past the box, for the rings and sparks around the core
const REST = 0.18 // at rest it breathes and drifts - typing, thinking or writing brings it up to full
const IDLE_MS = 40

// Petals around the core: softly irregular shapes, each with its own lobes, colour, turn and pulse
const PETALS = [
  { color: [255, 106, 61], lobes: 3, depth: 0.1, base: 0.66, turn: 0.15, pulse: 1.3, phase: 0 },
  { color: [234, 52, 57], lobes: 4, depth: 0.08, base: 0.7, turn: -0.11, pulse: 1.05, phase: 1.9 },
  { color: [255, 45, 111], lobes: 3, depth: 0.12, base: 0.63, turn: -0.19, pulse: 1.5, phase: 3.4 },
  { color: [255, 154, 107], lobes: 5, depth: 0.06, base: 0.72, turn: 0.08, pulse: 0.9, phase: 4.6 },
]

type Spark = { angle: number; reach: number; size: number; speed: number; phase: number }

// The same sparks every time - a seeded scatter in a band around the core
function sparks(n: number): Spark[] {
  let seed = 1013904223
  const rand = () => {
    seed = (seed * 1664525 + 1013904223) >>> 0
    return seed / 4294967296
  }
  return Array.from({ length: n }, () => ({ angle: rand() * 2 * Math.PI, reach: 0.95 + rand() * 0.6, size: 0.6 + rand() * 0.9, speed: 1 + rand() * 3, phase: rand() * 6 }))
}

const rgba = ([r, g, b]: readonly number[], a: number) => `rgba(${r}, ${g}, ${b}, ${a})`

function drawVoice(ctx: CanvasRenderingContext2D, px: number, detail: (typeof DETAIL)[keyof typeof DETAIL], dust: Spark[], t: number, amp: number, dpr: number) {
  const side = ctx.canvas.width
  const c = side / 2
  const R = px / 2
  ctx.clearRect(0, 0, side, side)

  if (detail.rings) {
    ctx.lineWidth = 0.6 * dpr
    for (const [k, a] of [
      [1.12, 0.16],
      [1.42, 0.1],
    ] as const) {
      ctx.strokeStyle = rgba([234, 52, 57], a)
      ctx.beginPath()
      ctx.arc(c, c, R * k * (1 + 0.03 * amp * Math.sin(1.4 * t + k)), 0, 2 * Math.PI)
      ctx.stroke()
    }
  }

  for (const d of dust) {
    const a = d.angle + 0.05 * t
    const glow = 0.25 + 0.6 * (0.5 + 0.5 * Math.sin(d.speed * t + d.phase))
    ctx.fillStyle = rgba([255, 90, 95], glow * (0.6 + 0.4 * amp))
    const sz = d.size * dpr
    ctx.beginPath()
    ctx.arc(c + Math.cos(a) * R * d.reach, c + Math.sin(a) * R * d.reach, sz / 2, 0, 2 * Math.PI)
    ctx.fill()
  }

  for (const p of PETALS.slice(0, detail.layers)) {
    const scale = 1 + (0.012 + 0.11 * amp) * Math.sin(p.pulse * (1 + amp) * t + p.phase)
    const turn = p.turn * t + p.phase
    ctx.beginPath()
    for (let i = 0; i <= 72; i++) {
      const th = (i / 72) * 2 * Math.PI
      const wobble = p.depth * (0.75 + 0.9 * amp) * Math.sin(p.lobes * th + turn) + 0.03 * Math.sin(2 * th - 0.7 * t + p.phase)
      const r = R * p.base * scale * (1 + wobble)
      const x = c + r * Math.cos(th)
      const y = c + r * Math.sin(th)
      if (i) ctx.lineTo(x, y)
      else ctx.moveTo(x, y)
    }
    ctx.closePath()
    ctx.fillStyle = rgba(p.color, 0.16)
    ctx.fill()
    ctx.strokeStyle = rgba(p.color, 0.85)
    ctx.lineWidth = Math.max(0.6, 0.8 * dpr)
    ctx.shadowColor = rgba(p.color, 0.6)
    ctx.shadowBlur = 4 * dpr
    ctx.stroke()
    ctx.shadowBlur = 0
  }

  const core = R * 0.46 * (1 + (0.012 + 0.07 * amp) * Math.sin(2.2 * t))
  const light = ctx.createRadialGradient(c, c, 0, c, c, core)
  light.addColorStop(0, 'rgba(255, 255, 255, 1)')
  light.addColorStop(0.45, 'rgba(255, 220, 222, 0.95)')
  light.addColorStop(0.8, 'rgba(255, 110, 115, 0.85)')
  light.addColorStop(1, 'rgba(234, 52, 57, 0.6)')
  ctx.fillStyle = light
  ctx.shadowColor = 'rgba(255, 80, 90, 0.8)'
  ctx.shadowBlur = 8 * dpr
  ctx.beginPath()
  ctx.arc(c, c, core, 0, 2 * Math.PI)
  ctx.fill()
  ctx.shadowBlur = 0
}

// The assistant's voice: a glowing core in petals of light that pulse deeper and quicker while it answers or thinks, only
// while on screen - still ones (decoration that repeats down a page) draw once
export function Presence({ speaking = false, still = false, size = 'lg', className }: { speaking?: boolean; still?: boolean; size?: keyof typeof SIZES; className?: string }) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const halo = useRef<HTMLSpanElement>(null)
  const amp = useRef(REST)
  const clock = useRef(0)
  const talking = useRef(speaking)
  const dots = useMemo(() => sparks(DETAIL[size].sparks), [size])
  useEffect(() => {
    talking.current = speaking
  }, [speaking])
  useEffect(() => {
    const el = canvas.current
    const ctx = el?.getContext('2d')
    if (!el || !ctx) return
    const dpr = Math.min(2, window.devicePixelRatio || 1)
    const side = Math.round(SIZES[size] * SPREAD * dpr)
    if (el.width !== side) {
      el.width = side
      el.height = side
    }
    const draw = () => drawVoice(ctx, SIZES[size] * dpr, DETAIL[size], dots, clock.current, amp.current, dpr)
    if (still || calm()) {
      draw()
      return
    }
    let frame = 0
    let visible = true
    let last = performance.now()
    const tick = (now: number) => {
      const speak = talking.current
      if (!speak && amp.current - REST < 0.005 && now - last < IDLE_MS) {
        frame = visible ? requestAnimationFrame(tick) : 0
        return
      }
      const dt = Math.min(0.1, (now - last) / 1000)
      last = now
      amp.current += ((speak ? 1 : REST) - amp.current) * Math.min(1, dt * (speak ? 5 : 2.5))
      clock.current += dt * (0.22 + 1.6 * amp.current)
      draw()
      if (halo.current) halo.current.style.opacity = String(0.1 + 0.3 * amp.current)
      frame = visible ? requestAnimationFrame(tick) : 0
    }
    const seen =
      typeof IntersectionObserver === 'function'
        ? new IntersectionObserver(([entry]) => {
            visible = entry.isIntersecting
            if (visible && !frame) {
              last = performance.now()
              frame = requestAnimationFrame(tick)
            }
          })
        : null
    seen?.observe(el)
    draw()
    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
      seen?.disconnect()
    }
  }, [size, dots, still])
  return (
    <span aria-hidden className={cn('presence relative block shrink-0 rounded-full', BOX[size], className)}>
      <span ref={halo} className="presence__halo" />
      <canvas ref={canvas} className="presence__voice" />
    </span>
  )
}

export function Streamed({ text, streaming }: { text: string; streaming: boolean }) {
  return (
    <>
      {text}
      {streaming ? <span className="aurora-caret" aria-hidden /> : null}
    </>
  )
}
