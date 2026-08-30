import { describe, expect, it } from 'vitest'
import {
  GROUND_Y,
  MAX_SPEED,
  RUNNER_X,
  START_SPEED,
  VIEW_W,
  createWorld,
  runnerBox,
  score,
  startRun,
  step,
  type Input,
  type World,
} from './engine'

// deterministic LCG so spawn sequences are reproducible across runs
function seeded(seed = 1): () => number {
  let s = seed
  return () => {
    s = (s * 1103515245 + 12345) % 2147483648
    return s / 2147483648
  }
}

const IDLE: Input = { jump: false, duck: false }

function run(w: World, frames: number, input: Input = IDLE) {
  for (let i = 0; i < frames; i++) step(w, input)
}

describe('boneyard engine', () => {
  it('only moves once started, and stops moving when dead', () => {
    const w = createWorld(seeded())
    step(w, { jump: false, duck: false })
    expect(w.t).toBe(0)
    expect(w.status).toBe('idle')

    startRun(w)
    step(w, IDLE)
    expect(w.t).toBe(1)

    w.status = 'dead'
    step(w, IDLE)
    expect(w.t).toBe(1)
  })

  it('jump leaves the ground and lands back exactly on it', () => {
    const w = createWorld(seeded())
    startRun(w)
    step(w, { jump: true, duck: false })
    expect(w.air).toBeGreaterThan(0)

    let frames = 0
    while (w.air > 0 && frames < 120) {
      step(w, IDLE)
      frames++
    }
    expect(w.air).toBe(0)
    expect(w.vy).toBe(0)
    expect(frames).toBeGreaterThan(20)
    expect(frames).toBeLessThan(60)
  })

  it('holding down mid-air drops faster than a free fall', () => {
    const jumpThenFall = (duck: boolean) => {
      const w = createWorld(seeded())
      startRun(w)
      step(w, { jump: true, duck: false })
      let frames = 0
      while (w.air > 0 && frames < 120) {
        step(w, { jump: false, duck })
        frames++
      }
      return frames
    }
    expect(jumpThenFall(true)).toBeLessThan(jumpThenFall(false))
  })

  it('ducking shrinks the hitbox and stands back up on release', () => {
    const w = createWorld(seeded())
    startRun(w)
    const standing = runnerBox(w)
    step(w, { jump: false, duck: true })
    const ducked = runnerBox(w)
    expect(w.ducking).toBe(true)
    expect(ducked.h).toBeLessThan(standing.h)
    expect(ducked.y).toBeGreaterThan(standing.y)
    step(w, IDLE)
    expect(w.ducking).toBe(false)
    expect(runnerBox(w).h).toBe(standing.h)
  })

  it('speed accelerates from the start value and caps at the max', () => {
    const w = createWorld(seeded())
    startRun(w)
    w.obstacles.length = 0
    for (let i = 0; i < 500; i++) {
      step(w, IDLE)
      w.obstacles.length = 0 // scoring test only - clear the course
    }
    expect(w.speed).toBeGreaterThan(START_SPEED)
    expect(score(w)).toBe(Math.floor(w.distance * 0.025))

    w.speed = MAX_SPEED
    step(w, IDLE)
    expect(w.speed).toBe(MAX_SPEED)
  })

  it('spawns obstacles off-screen, scrolls them left and culls them', () => {
    const w = createWorld(seeded(7))
    startRun(w)
    let frames = 0
    while (w.obstacles.length === 0 && frames < 300) {
      step(w, IDLE)
      frames++
    }
    const first = w.obstacles[0]
    expect(first).toBeDefined()
    expect(first.x).toBeGreaterThanOrEqual(VIEW_W)

    const xBefore = first.x
    step(w, IDLE)
    expect(first.x).toBeLessThan(xBefore)

    let sawOnePass = false
    for (let i = 0; i < 1500; i++) {
      // hover far above the course so nothing can collide
      w.air = 500
      w.vy = 0
      step(w, IDLE)
      for (const o of w.obstacles) {
        expect(o.x + o.w).toBeGreaterThan(-16)
        if (o.x + o.w < RUNNER_X) sawOnePass = true
      }
    }
    expect(w.status).toBe('running')
    expect(sawOnePass).toBe(true)
    expect(w.obstacles.length).toBeGreaterThan(0)
    expect(w.obstacles.length).toBeLessThan(10)
  })

  it('dies on contact with an obstacle', () => {
    const w = createWorld(seeded())
    startRun(w)
    w.obstacles.push({ kind: 'graveSmall', x: RUNNER_X, y: GROUND_Y - 32, w: 24, h: 32, vx: 0 })
    step(w, IDLE)
    expect(w.status).toBe('dead')
  })

  it('clears a ground bat by jumping over it', () => {
    const w = createWorld(seeded())
    startRun(w)
    // apex of the jump, bat passing underneath
    step(w, { jump: true, duck: false })
    run(w, 17)
    expect(w.air).toBeGreaterThan(50)
    w.obstacles.push({ kind: 'bat', x: RUNNER_X, y: GROUND_Y - 34, w: 36, h: 24, vx: 0 })
    step(w, IDLE)
    expect(w.status).toBe('running')
  })

  it('a mid-height bat hits a standing runner but misses a ducking one', () => {
    const standing = createWorld(seeded())
    startRun(standing)
    standing.obstacles.push({ kind: 'bat', x: RUNNER_X, y: GROUND_Y - 52, w: 36, h: 24, vx: 0 })
    step(standing, IDLE)
    expect(standing.status).toBe('dead')

    const ducking = createWorld(seeded())
    startRun(ducking)
    step(ducking, { jump: false, duck: true })
    ducking.obstacles.push({ kind: 'bat', x: RUNNER_X, y: GROUND_Y - 52, w: 36, h: 24, vx: 0 })
    step(ducking, { jump: false, duck: true })
    expect(ducking.status).toBe('running')
  })

  it('is deterministic under the same seed and inputs', () => {
    const play = () => {
      const w = createWorld(seeded(42))
      startRun(w)
      for (let i = 0; i < 600 && w.status === 'running'; i++) {
        step(w, { jump: i % 45 === 0, duck: false })
      }
      return JSON.stringify(w) // rand drops out of JSON, everything else must match
    }
    expect(play()).toBe(play())
  })

  it('restarting resets the course but keeps nothing stale around', () => {
    const w = createWorld(seeded())
    startRun(w)
    run(w, 200)
    w.status = 'dead'
    startRun(w)
    expect(w.status).toBe('running')
    expect(w.t).toBe(0)
    expect(w.distance).toBe(0)
    expect(w.obstacles).toEqual([])
  })
})
