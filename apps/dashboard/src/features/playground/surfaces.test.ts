import { afterEach, describe, expect, it } from 'vitest'
import { namespacedStorage, resetSurfaceTabs } from './surfaces'

describe('resetSurfaceTabs', () => {
  afterEach(() => window.localStorage.clear())

  it("drops one surface's tabs and editors, and keeps its preferences and the other surface", () => {
    const front = namespacedStorage('front')
    const admin = namespacedStorage('admin')
    for (const key of ['tabState', 'query', 'variables', 'headers', 'theme', 'visiblePlugin', 'shouldPersistHeaders']) front.setItem(`graphiql:${key}`, 'x')
    admin.setItem('graphiql:tabState', 'admin tabs')

    resetSurfaceTabs('front')

    expect(['tabState', 'query', 'variables', 'headers'].map((k) => front.getItem(`graphiql:${k}`))).toEqual([null, null, null, null])
    expect(['theme', 'visiblePlugin', 'shouldPersistHeaders'].map((k) => front.getItem(`graphiql:${k}`))).toEqual(['x', 'x', 'x'])
    expect(admin.getItem('graphiql:tabState')).toBe('admin tabs')
  })
})
