import { describe, expect, it } from 'vitest'
import lockfile from '../../../pnpm-lock.yaml?raw'

// What this pins: the playground's own panels and GraphiQL share one @graphiql/react. A lockfile refresh once moved
// graphiql to a release needing a newer one while ours stayed put, so the panels read a store GraphiQL never filled and
// opening one replaced the whole dashboard with an error page
describe('the GraphiQL packages', () => {
  it('lock a single version of @graphiql/react', () => {
    const versions = new Set([...lockfile.matchAll(/^ {2}'@graphiql\/react@([^'(]+)'/gm)].map((m) => m[1]))
    expect([...versions]).toHaveLength(1)
  })
})
