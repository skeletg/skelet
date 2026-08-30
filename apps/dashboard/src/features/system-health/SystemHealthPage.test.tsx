import { render, screen } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it } from 'vitest'

import { SystemHealthDocument } from './SystemHealth.gql'
import { SystemHealthPage } from './SystemHealthPage'
import { meaningful, type Check } from './refreshCue'

// The page renders whatever groups the backend registered, so the fixture is shaped
// like a real report rather than mirroring any particular subsystem
const report = {
  systemHealth: {
    __typename: 'SystemHealth',
    status: 'BAD',
    checkedAt: new Date().toISOString(),
    groups: [
      {
        __typename: 'HealthGroup',
        key: 'datastores',
        label: 'Datastores',
        checks: [
          {
            __typename: 'HealthCheck',
            key: 'db',
            label: 'Commerce database',
            status: 'OK',
            detail: 'reachable - 4/25 connections in use',
            hint: null,
          },
          {
            __typename: 'HealthCheck',
            key: 'cache',
            label: 'Cache (Valkey)',
            status: 'OFF',
            detail: 'not configured',
            hint: 'Sessions stop being shared across pods',
          },
        ],
      },
      {
        __typename: 'HealthGroup',
        key: 'signals',
        label: 'Signals pipeline',
        checks: [
          {
            __typename: 'HealthCheck',
            key: 'queue',
            label: 'Ingest queue',
            status: 'BAD',
            detail: 'no consumer, 12043 in stream',
            hint: 'Nothing is draining the queue',
          },
        ],
      },
    ],
  },
}

const mocks = [{ request: { query: SystemHealthDocument }, result: { data: report } }]

function renderPage() {
  return render(
    <MockedProvider mocks={mocks}>
      <SystemHealthPage />
    </MockedProvider>,
  )
}

describe('SystemHealthPage', () => {
  it('headlines the worst status across every group', async () => {
    renderPage()
    expect(await screen.findByText('Something is broken')).toBeDefined()
  })

  it('counts checks by status, with OFF kept separate from failures', async () => {
    renderPage()
    await screen.findByText('Something is broken')
    const tile = (label: string) =>
      screen.getByText(label).closest('div')?.parentElement?.textContent ?? ''
    expect(tile('Passing')).toContain('1')
    expect(tile('Broken')).toContain('1')
    expect(tile('Not configured')).toContain('1')
  })

  // Nothing is ever hidden behind a disclosure - every check the backend reported is
  // on the page, healthy ones included
  it('shows every check from every group at once', async () => {
    renderPage()
    expect(await screen.findByText('Nothing is draining the queue')).toBeDefined()
    expect(screen.getByText('reachable - 4/25 connections in use')).toBeDefined()
    expect(screen.getByText('not configured')).toBeDefined()
  })

  it('rolls a group up in its header as well as listing it', async () => {
    renderPage()
    await screen.findByText('Something is broken')
    expect(screen.getByText('1 passing, 1 off')).toBeDefined()
  })
})

// The refresh wash has to fire on something an operator would care about and stay
// quiet otherwise - at a 5s poll, getting this wrong washes the page constantly
describe('meaningful', () => {
  const base: Check = { key: 'ingest', label: 'Event ingest', status: 'OK', detail: 'last event 12s ago', hint: null }

  it('ignores relative-time drift, which changes on every single poll', () => {
    expect(meaningful({ ...base, detail: 'last event 17s ago' })).toBe(meaningful(base))
    expect(meaningful({ ...base, detail: 'oldest event 3d ago, keeping 30d' })).toBe(
      meaningful({ ...base, detail: 'oldest event 4d ago, keeping 30d' }),
    )
  })

  it('still notices a real change behind a relative time', () => {
    expect(meaningful({ ...base, detail: '31h ago (7 kept)' })).not.toBe(
      meaningful({ ...base, detail: '31h ago (6 kept)' }),
    )
  })

  it('notices status and value changes', () => {
    expect(meaningful({ ...base, status: 'WARN' })).not.toBe(meaningful(base))
    expect(meaningful({ ...base, detail: 'no consumer, 12043 in stream' })).not.toBe(
      meaningful({ ...base, detail: 'no consumer, 18902 in stream' }),
    )
  })
})
