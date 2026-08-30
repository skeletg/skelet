// Pure endless-runner simulation - no DOM, deterministic under an injected
// rand, so the whole thing is unit-testable; SkeletonRun.tsx does the drawing

export const VIEW_W = 600
export const VIEW_H = 180
export const GROUND_Y = 160
export const RUNNER_X = 42

export const STAND_W = 44
export const STAND_H = 44
export const DUCK_W = 60
export const DUCK_H = 28

export const START_SPEED = 6
export const MAX_SPEED = 12.5
const ACCEL = 0.0012
const GRAVITY = 0.55
const DROP_PULL = 0.9 // extra pull while holding down mid-air
const JUMP_V = 10.3
const SCORE_PER_PX = 0.025
const BAT_MIN_SPEED = 8
const BAT_EXTRA_VX = 0.8
const FIRST_GAP = 420

export type Status = 'idle' | 'running' | 'dead'
export type ObstacleKind = 'graveSmall' | 'graveTall' | 'graveCluster' | 'bat'

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface Obstacle extends Rect {
  kind: ObstacleKind
  vx: number
}

export interface Cloud {
  x: number
  y: number
}

export interface Input {
  jump: boolean
  duck: boolean
}

export interface World {
  status: Status
  t: number
  speed: number
  distance: number
  air: number // feet height above the ground - 0 means grounded
  vy: number
  ducking: boolean
  obstacles: Obstacle[]
  clouds: Cloud[]
  untilSpawn: number // px of travel left before the next obstacle
  untilCloud: number // frames until the next cloud
  flashUntil: number // score blinks until this frame after a century mark
  lastMilestone: number
  rand: () => number
}

export function createWorld(rand: () => number = Math.random): World {
  return {
    status: 'idle',
    t: 0,
    speed: START_SPEED,
    distance: 0,
    air: 0,
    vy: 0,
    ducking: false,
    obstacles: [],
    clouds: [],
    untilSpawn: FIRST_GAP,
    untilCloud: 40,
    flashUntil: 0,
    lastMilestone: 0,
    rand,
  }
}

export function startRun(w: World): void {
  Object.assign(w, createWorld(w.rand), { status: 'running' })
}

export function score(w: World): number {
  return Math.floor(w.distance * SCORE_PER_PX)
}

function inset(r: Rect, ix: number, iy: number): Rect {
  return { x: r.x + ix, y: r.y + iy, w: r.w - ix * 2, h: r.h - iy * 2 }
}

function overlaps(a: Rect, b: Rect): boolean {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y
}

// Hitboxes are inset from the sprites so grazing a pixel feels fair
export function runnerBox(w: World): Rect {
  const width = w.ducking ? DUCK_W : STAND_W
  const height = w.ducking ? DUCK_H : STAND_H
  const feet = GROUND_Y - w.air
  return inset({ x: RUNNER_X, y: feet - height, w: width, h: height }, w.ducking ? 10 : 8, w.ducking ? 4 : 5)
}

export function obstacleBox(o: Obstacle): Rect {
  return o.kind === 'bat' ? inset(o, 6, 5) : inset(o, 3, 2)
}

const GRAVE_SIZES: Record<Exclude<ObstacleKind, 'bat'>, { w: number; h: number }> = {
  graveSmall: { w: 24, h: 32 },
  graveTall: { w: 24, h: 44 },
  graveCluster: { w: 48, h: 32 },
}

function spawnObstacle(w: World): void {
  const batShare = w.speed >= BAT_MIN_SPEED ? 0.22 : 0
  const r = w.rand()
  if (r < batShare) {
    // three flight levels: jump it, duck it, or leave it alone
    const lv = w.rand()
    const top = lv < 0.4 ? GROUND_Y - 34 : lv < 0.8 ? GROUND_Y - 52 : GROUND_Y - 92
    w.obstacles.push({ kind: 'bat', x: VIEW_W + 8, y: top, w: 36, h: 24, vx: BAT_EXTRA_VX })
  } else {
    const kind: ObstacleKind =
      r < batShare + 0.36 ? 'graveSmall' : r < batShare + 0.64 ? 'graveTall' : 'graveCluster'
    const size = GRAVE_SIZES[kind as Exclude<ObstacleKind, 'bat'>]
    w.obstacles.push({ kind, x: VIEW_W + 8, y: GROUND_Y - size.h, w: size.w, h: size.h, vx: 0 })
  }
  w.untilSpawn = (240 + w.speed * 24) * (0.72 + w.rand() * 0.6)
}

export function step(w: World, input: Input): void {
  if (w.status !== 'running') return
  w.t++
  w.speed = Math.min(MAX_SPEED, w.speed + ACCEL)
  w.distance += w.speed

  if (input.jump && w.air === 0 && w.vy === 0) w.vy = JUMP_V
  const airborne = w.air > 0 || w.vy !== 0
  w.ducking = !airborne && input.duck
  if (airborne) {
    w.vy -= GRAVITY + (input.duck ? DROP_PULL : 0)
    w.air += w.vy
    if (w.air <= 0) {
      w.air = 0
      w.vy = 0
    }
  }

  w.untilSpawn -= w.speed
  if (w.untilSpawn <= 0) spawnObstacle(w)
  for (const o of w.obstacles) o.x -= w.speed + o.vx
  w.obstacles = w.obstacles.filter((o) => o.x + o.w > -16)

  if (--w.untilCloud <= 0) {
    w.clouds.push({ x: VIEW_W + 8, y: 18 + w.rand() * 70 })
    w.untilCloud = 130 + Math.floor(w.rand() * 220)
  }
  for (const c of w.clouds) c.x -= w.speed * 0.25
  w.clouds = w.clouds.filter((c) => c.x > -40)

  const milestone = Math.floor(score(w) / 100)
  if (milestone > w.lastMilestone) {
    w.lastMilestone = milestone
    w.flashUntil = w.t + 70
  }

  const rb = runnerBox(w)
  if (w.obstacles.some((o) => overlaps(rb, obstacleBox(o)))) w.status = 'dead'
}
