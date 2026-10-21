import { act, fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { meMock, renderPage } from '@/test/page-harness'
import { CmsPagesDocument, CmsTagsDocument } from './Cms.gql'
import { CmsPagesPage } from './CmsPagesPage'

// What this pins: the page search refetches only once typing has paused for 300ms, and sends the
// term trimmed

afterEach(() => vi.useRealTimers())

describe('CmsPagesPage', () => {
  it('searches 300ms after typing stops, trimmed', async () => {
    vi.useFakeTimers()
    const asked: (string | undefined)[] = []
    const mocks = [
      meMock(),
      { request: { query: CmsTagsDocument }, result: { data: { cmsTags: [] } }, maxUsageCount: 5 },
      {
        request: {
          query: CmsPagesDocument,
          variables: (vars: { filter: { search?: string } }) => {
            asked.push(vars.filter.search)
            return true
          },
        },
        result: { data: { cmsPages: [] } },
        maxUsageCount: 5,
      },
    ]
    renderPage({ path: '/cms/pages', routes: { '/cms/pages': <CmsPagesPage /> }, mocks, adminId: 'admin-1' })
    await act(async () => {})
    expect(asked).toEqual([undefined])

    fireEvent.change(screen.getByPlaceholderText(/^Search title/), { target: { value: '  tee  ' } })
    await act(async () => vi.advanceTimersByTimeAsync(299))
    expect(asked).toEqual([undefined])
    await act(async () => vi.advanceTimersByTimeAsync(1))
    expect(asked).toEqual([undefined, 'tee'])
  })
})
