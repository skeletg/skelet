import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CacheAdminPage } from './CacheAdminPage'
import { CacheRevalidationsDocument, CacheSurfacesDocument, PurgeCacheDocument, RebuildQueueDocument } from './Cache.gql'

// What this pins: "Invalidate everything" stales the whole storefront at once, so it asks first and purges only on confirm

describe('CacheAdminPage invalidate everything', () => {
  it('purges every surface only after the admin confirms', async () => {
    const purged = vi.fn()
    renderPage({
      path: '/cache',
      routes: { '/cache': <CacheAdminPage /> },
      mocks: [
        meMock(),
        { request: { query: CacheSurfacesDocument }, result: { data: { cacheSurfaces: [] } }, maxUsageCount: 3 },
        { request: { query: CacheRevalidationsDocument, variables: { limit: 50 } }, result: { data: { cacheRevalidations: [] } }, maxUsageCount: 3 },
        {
          request: { query: RebuildQueueDocument, variables: { recentLimit: 10 } },
          result: { data: { rebuildQueue: { __typename: 'RebuildQueue', pending: 0, running: 0, done: 0, failed: 0, recent: [] } } },
          maxUsageCount: 3,
        },
        {
          request: {
            query: PurgeCacheDocument,
            variables: (vars: Record<string, unknown>) => {
              purged(vars)
              return true
            },
          },
          result: { data: { purgeCache: true } },
        },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: /Invalidate everything/ }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(purged).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Invalidate everything' }))
    await vi.waitFor(() => expect(purged).toHaveBeenCalledWith({}), SLOW)
  })
})
