import { act, fireEvent, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderDialog } from '@/test/page-harness'
import { LinkablesDocument } from './Cms.gql'
import { LinkField } from './LinkField'

// What this pins: the internal-link search queries linkables only once typing has paused for 250ms

Element.prototype.scrollIntoView ??= () => {}

afterEach(() => vi.useRealTimers())

describe('LinkField', () => {
  it('searches linkables 250ms after typing stops', async () => {
    vi.useFakeTimers()
    const asked: string[] = []
    const mocks = [
      {
        request: {
          query: LinkablesDocument,
          variables: (vars: { search: string }) => {
            asked.push(vars.search)
            return true
          },
        },
        result: { data: { linkables: [] } },
        maxUsageCount: 5,
      },
    ]
    const ref = { type: 'reference', entityType: 'cms_page', entityId: 'pg1', label: 'About' }
    renderDialog(<LinkField value={ref} onChange={() => {}} disabled={false} />, mocks)
    await act(async () => {})
    expect(asked).toEqual([''])

    fireEvent.click(screen.getByRole('combobox'))
    fireEvent.change(screen.getByPlaceholderText('Search…'), { target: { value: 'tee' } })
    await act(async () => vi.advanceTimersByTimeAsync(249))
    expect(asked).toEqual([''])
    await act(async () => vi.advanceTimersByTimeAsync(1))
    expect(asked).toEqual(['', 'tee'])
  })
})
