import { useEffect, useRef } from 'react'
import {
  GROUND_Y,
  RUNNER_X,
  DUCK_H,
  STAND_H,
  VIEW_H,
  VIEW_W,
  createWorld,
  score,
  startRun,
  step,
  type Obstacle,
  type World,
} from './engine'
import {
  BAT_A,
  BAT_B,
  CLOUD,
  GRAVE_SMALL,
  GRAVE_TALL,
  ROUND_STONE,
  SCALE,
  SKELETON_DEAD,
  SKELETON_DUCK_A,
  SKELETON_DUCK_B,
  SKELETON_JUMP,
  SKELETON_RUN_A,
  SKELETON_RUN_B,
  type Sprite,
} from './sprites'

const HI_KEY = 'skelet.boneyard.hi'
const FRAME_MS = 1000 / 60
const HUD_FONT = '700 13px ui-monospace, SFMono-Regular, Menlo, monospace'

type Palette = { fg: string; muted: string }

// Colors come from the live theme tokens, so the game matches every palette
function readPalette(el: HTMLElement): Palette {
  const cs = getComputedStyle(el)
  return {
    fg: cs.getPropertyValue('--foreground').trim() || '#52525b',
    muted: cs.getPropertyValue('--muted-foreground').trim() || '#a1a1aa',
  }
}

const rasterCache = new WeakMap<Sprite, Map<string, HTMLCanvasElement>>()

function raster(sprite: Sprite, color: string): HTMLCanvasElement {
  let byColor = rasterCache.get(sprite)
  if (!byColor) {
    byColor = new Map()
    rasterCache.set(sprite, byColor)
  }
  const hit = byColor.get(color)
  if (hit) return hit
  const c = document.createElement('canvas')
  c.width = (sprite[0]?.length ?? 0) * SCALE
  c.height = sprite.length * SCALE
  const g = c.getContext('2d')
  if (g) {
    g.fillStyle = color
    sprite.forEach((row, y) => {
      for (let x = 0; x < row.length; x++) {
        if (row[x] === '#') g.fillRect(x * SCALE, y * SCALE, SCALE, SCALE)
      }
    })
  }
  byColor.set(color, c)
  return c
}

function hash01(n: number): number {
  return Math.abs(Math.sin(n * 127.1) * 43758.5453) % 1
}

function obstacleSprites(o: Obstacle, t: number): Array<{ sprite: Sprite; dx: number; dy: number }> {
  switch (o.kind) {
    case 'bat':
      return [{ sprite: (t >> 4) & 1 ? BAT_B : BAT_A, dx: 0, dy: 0 }]
    case 'graveSmall':
      return [{ sprite: GRAVE_SMALL, dx: 0, dy: 0 }]
    case 'graveTall':
      return [{ sprite: GRAVE_TALL, dx: 0, dy: 0 }]
    case 'graveCluster':
      return [
        { sprite: GRAVE_SMALL, dx: 0, dy: 0 },
        { sprite: ROUND_STONE, dx: 28, dy: 12 },
      ]
  }
}

function runnerSprite(w: World): Sprite {
  if (w.status === 'dead') return SKELETON_DEAD
  if (w.air > 0 || w.vy !== 0) return SKELETON_JUMP
  const alt = ((w.t >> 3) & 1) === 1
  if (w.ducking) return alt ? SKELETON_DUCK_B : SKELETON_DUCK_A
  if (w.status === 'idle') return SKELETON_RUN_A
  return alt ? SKELETON_RUN_B : SKELETON_RUN_A
}

function readHi(): number {
  const raw = Number(localStorage.getItem(HI_KEY) ?? '0')
  return Number.isFinite(raw) && raw > 0 ? Math.floor(raw) : 0
}

export function SkeletonRun({ autoStart = false }: { autoStart?: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return

    const dpr = Math.min(2, window.devicePixelRatio || 1)
    canvas.width = VIEW_W * dpr
    canvas.height = VIEW_H * dpr

    let world = createWorld()
    if (autoStart) startRun(world)
    let hi = readHi()
    let diedAt = 0
    let palette = readPalette(canvas)
    const input = { jumpQueued: false, duckHeld: false }

    const themeWatch = new MutationObserver(() => {
      palette = readPalette(canvas)
    })
    themeWatch.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class', 'style', 'data-theme'],
    })

    const advance = (jump: boolean) => {
      if (world.status === 'idle' && jump) {
        startRun(world)
        return
      }
      if (world.status === 'dead') {
        // small grace so the losing jump-mash doesn't insta-restart
        if (jump && performance.now() - diedAt > 300) {
          world = createWorld()
          startRun(world)
        }
        return
      }
      step(world, { jump, duck: input.duckHeld })
      // step() mutates status, but TS property narrowing survives the call - the
      // cast re-widens so the death check isn't flagged as an impossible comparison
      if ((world as World).status === 'dead') {
        diedAt = performance.now()
        const s = score(world)
        if (s > hi) {
          hi = s
          try {
            localStorage.setItem(HI_KEY, String(hi))
          } catch {
            // storage full or blocked - the run still counts
          }
        }
      }
    }

    const draw = () => {
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, VIEW_W, VIEW_H)
      ctx.imageSmoothingEnabled = false

      ctx.globalAlpha = 0.5
      for (const c of world.clouds) ctx.drawImage(raster(CLOUD, palette.muted), Math.round(c.x), Math.round(c.y))
      ctx.globalAlpha = 1

      ctx.fillStyle = palette.fg
      ctx.fillRect(0, GROUND_Y + 1, VIEW_W, 2)
      ctx.fillStyle = palette.muted
      for (let i = 0; i <= VIEW_W / 24 + 1; i++) {
        const idx = Math.floor(world.distance / 24) + i
        const h = hash01(idx)
        if (h < 0.55) {
          ctx.fillRect(i * 24 - (world.distance % 24) + h * 16, GROUND_Y + 7 + h * 8, 3 + Math.floor(h * 6), 2)
        }
      }

      for (const o of world.obstacles) {
        for (const part of obstacleSprites(o, world.t)) {
          ctx.drawImage(raster(part.sprite, palette.fg), Math.round(o.x + part.dx), Math.round(o.y + part.dy))
        }
      }

      const feet = GROUND_Y - world.air
      const top = feet - (world.ducking ? DUCK_H : STAND_H)
      ctx.drawImage(raster(runnerSprite(world), palette.fg), RUNNER_X, Math.round(top))

      ctx.font = HUD_FONT
      ctx.textAlign = 'right'
      ctx.textBaseline = 'top'
      const s = String(score(world)).padStart(5, '0')
      const flashing = world.t < world.flashUntil && ((world.t >> 3) & 1) === 1
      if (!flashing) {
        ctx.fillStyle = palette.fg
        ctx.fillText(s, VIEW_W - 8, 10)
      }
      if (hi > 0) {
        ctx.fillStyle = palette.muted
        ctx.fillText(`HI ${String(hi).padStart(5, '0')}`, VIEW_W - 68, 10)
      }

      ctx.textAlign = 'center'
      if (world.status === 'idle') {
        ctx.fillStyle = palette.muted
        ctx.fillText('PRESS SPACE OR TAP TO RUN', VIEW_W / 2, 62)
      } else if (world.status === 'dead') {
        ctx.fillStyle = palette.fg
        ctx.font = '700 15px ui-monospace, SFMono-Regular, Menlo, monospace'
        ctx.fillText('G A M E  O V E R', VIEW_W / 2, 56)
        ctx.strokeStyle = palette.fg
        ctx.lineWidth = 3
        ctx.beginPath()
        ctx.arc(VIEW_W / 2, 96, 11, -0.35 * Math.PI, 1.15 * Math.PI)
        ctx.stroke()
        ctx.fillStyle = palette.fg
        ctx.beginPath()
        const tipX = VIEW_W / 2 + 11 * Math.cos(-0.35 * Math.PI)
        const tipY = 96 + 11 * Math.sin(-0.35 * Math.PI)
        ctx.moveTo(tipX + 5, tipY - 3)
        ctx.lineTo(tipX - 4, tipY - 5)
        ctx.lineTo(tipX + 2, tipY + 6)
        ctx.closePath()
        ctx.fill()
      }
    }

    let raf = 0
    let last = performance.now()
    let acc = 0
    const tick = (now: number) => {
      acc += Math.min(100, now - last)
      last = now
      while (acc >= FRAME_MS) {
        const jump = input.jumpQueued
        input.jumpQueued = false
        advance(jump)
        acc -= FRAME_MS
      }
      draw()
      raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)

    const isTypingTarget = (t: EventTarget | null) =>
      t instanceof HTMLElement && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)
    const onKeyDown = (e: KeyboardEvent) => {
      if (isTypingTarget(e.target)) return
      if (e.code === 'Space' || e.code === 'ArrowUp') {
        e.preventDefault()
        if (!e.repeat) input.jumpQueued = true
      } else if (e.code === 'ArrowDown') {
        e.preventDefault()
        input.duckHeld = true
      }
    }
    const onKeyUp = (e: KeyboardEvent) => {
      if (e.code === 'ArrowDown') input.duckHeld = false
    }
    const onPointerDown = (e: PointerEvent) => {
      e.preventDefault()
      input.jumpQueued = true
    }
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('keyup', onKeyUp)
    canvas.addEventListener('pointerdown', onPointerDown)

    return () => {
      cancelAnimationFrame(raf)
      themeWatch.disconnect()
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup', onKeyUp)
      canvas.removeEventListener('pointerdown', onPointerDown)
    }
  }, [autoStart])

  return (
    <canvas
      ref={canvasRef}
      width={VIEW_W}
      height={VIEW_H}
      className="block h-auto w-full max-w-[600px] touch-none select-none"
      role="img"
      aria-label="A skeleton sprinting through a graveyard, jumping headstones and ducking bats"
    />
  )
}
