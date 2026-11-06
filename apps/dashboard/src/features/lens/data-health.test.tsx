import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { renderPage } from '@/test/page-harness'
import { DataHealthSection } from './DataHealthSection'

// DataTable keeps its column and density preferences per admin, so the section renders as a page would
const show = (node: ReactNode) => renderPage({ path: '/', routes: { '/': node }, mocks: [], adminId: 'admin-1' })

describe('DataHealthSection', () => {
  it('lists what ingest dropped, with a readable issue', () => {
    show(
      <DataHealthSection
        issues={[
          { kind: 'SERVER_PROP', event: 'page_view', prop: 'revenue_cents', count: 1200 },
          { kind: 'UNKNOWN_EVENT', event: '$made_up', prop: '', count: 3 },
        ]}
      />,
    )
    expect(screen.getByText('Server-only property, dropped')).toBeTruthy()
    expect(screen.getByText('revenue_cents')).toBeTruthy()
    expect(screen.getByText('Unknown event, dropped')).toBeTruthy()
    expect(screen.getByText('$made_up')).toBeTruthy()
    expect(screen.getByText('1,200')).toBeTruthy()
  })

  it('says so when every event matched', () => {
    show(<DataHealthSection issues={[]} />)
    expect(screen.getByText('Every browser event matched the tracking plan.')).toBeTruthy()
  })
})
