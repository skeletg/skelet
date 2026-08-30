import { Suspense } from 'react'
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { lazyPage } from './lazy'

// Every route element goes through lazyPage (route-level code splitting), so its named-export shim and props
// pass-through are load-bearing for the whole router

describe('lazyPage', () => {
  it('resolves a named export and renders it', async () => {
    const el = lazyPage(
      () => Promise.resolve({ FakePage: () => <div>fake page</div> }),
      'FakePage',
    )
    const { findByText } = render(<Suspense fallback="loading">{el}</Suspense>)
    expect(await findByText('fake page')).toBeTruthy()
  })

  it('passes props through to the loaded component', async () => {
    const el = lazyPage(
      () =>
        Promise.resolve({
          Statuses: ({ defaultStatus }: { defaultStatus: string[] }) => (
            <div>status: {defaultStatus.join(',')}</div>
          ),
        }),
      'Statuses',
      { defaultStatus: ['PENDING', 'PAID'] },
    )
    const { findByText } = render(<Suspense fallback="loading">{el}</Suspense>)
    expect(await findByText('status: PENDING,PAID')).toBeTruthy()
  })
})
