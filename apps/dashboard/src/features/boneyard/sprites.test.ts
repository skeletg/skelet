import { describe, expect, it } from 'vitest'
import { DUCK_H, DUCK_W, STAND_H, STAND_W } from './engine'
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

const ALL: Record<string, Sprite> = {
  SKELETON_RUN_A,
  SKELETON_RUN_B,
  SKELETON_JUMP,
  SKELETON_DEAD,
  SKELETON_DUCK_A,
  SKELETON_DUCK_B,
  BAT_A,
  BAT_B,
  GRAVE_SMALL,
  GRAVE_TALL,
  ROUND_STONE,
  CLOUD,
}

describe('boneyard sprites', () => {
  it('every grid is rectangular and only uses the two legal cells', () => {
    for (const [name, sprite] of Object.entries(ALL)) {
      expect(sprite.length, name).toBeGreaterThan(0)
      const width = sprite[0].length
      for (const row of sprite) {
        expect(row.length, `${name} row width`).toBe(width)
        expect(row, `${name} cells`).toMatch(/^[.#]+$/)
      }
    }
  })

  it('animation frames share dimensions so swapping them cannot jitter', () => {
    const pairs: Array<[Sprite, Sprite]> = [
      [SKELETON_RUN_A, SKELETON_RUN_B],
      [SKELETON_RUN_A, SKELETON_JUMP],
      [SKELETON_RUN_A, SKELETON_DEAD],
      [SKELETON_DUCK_A, SKELETON_DUCK_B],
      [BAT_A, BAT_B],
    ]
    for (const [a, b] of pairs) {
      expect(a.length).toBe(b.length)
      expect(a[0].length).toBe(b[0].length)
    }
  })

  it('runner sprites match the hitbox dimensions the engine collides with', () => {
    expect(SKELETON_RUN_A[0].length * SCALE).toBe(STAND_W)
    expect(SKELETON_RUN_A.length * SCALE).toBe(STAND_H)
    expect(SKELETON_DUCK_A[0].length * SCALE).toBe(DUCK_W)
    expect(SKELETON_DUCK_A.length * SCALE).toBe(DUCK_H)
  })

  it('obstacle sprites match the sizes the engine spawns', () => {
    expect(GRAVE_SMALL[0].length * SCALE).toBe(24)
    expect(GRAVE_SMALL.length * SCALE).toBe(32)
    expect(GRAVE_TALL[0].length * SCALE).toBe(24)
    expect(GRAVE_TALL.length * SCALE).toBe(44)
    expect(BAT_A[0].length * SCALE).toBe(36)
    expect(BAT_A.length * SCALE).toBe(24)
    // cluster = small grave + round stone with a 4px gap
    expect(GRAVE_SMALL[0].length * SCALE + 4 + ROUND_STONE[0].length * SCALE).toBe(48)
    expect(ROUND_STONE.length * SCALE).toBe(20)
  })
})
