import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DataHealthSection } from './LensHome'

describe('DataHealthSection', () => {
  it('lists what ingest dropped, with a readable issue', () => {
    render(
      <DataHealthSection
        issues={[
          { kind: 'plan_server_prop', event: 'page_view', prop: 'revenue_cents', count: 1200 },
          { kind: 'plan_unknown_event', event: '$made_up', prop: '', count: 3 },
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
    render(<DataHealthSection issues={[]} />)
    expect(screen.getByText('Every browser event matched the tracking plan.')).toBeTruthy()
  })
})
