import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Reach, SendDate } from './SendSheet'

// What this pins: reach is said in words as well as drawn, and the set date reads as plain text to a screen reader

const reach = (props: Parameters<typeof Reach>[0]) => render(<TooltipProvider><Reach {...props} /></TooltipProvider>)

describe('who gets it', () => {
  it('splits the opted-in audience three ways for a subject test', () => {
    reach({ total: 1000, eligible: 400, split: { a: 40, b: 40, rest: 320 }, pending: '' })
    expect(screen.getByRole('img').getAttribute('aria-label')).toBe('Subject A: about 40, Subject B: about 40, The winner, after the test: about 320, Not opted in, skipped: 600')
    expect(screen.getAllByRole('listitem').map((li) => li.textContent)).toEqual(['Subject A~40', 'Subject B~40', 'The winner, after the test~320', 'Not opted in, skipped600'])
  })

  it('warns when nobody would get it, and waits for an audience to be picked', () => {
    const { unmount } = reach({ total: 12, eligible: 0, split: null, pending: '' })
    expect(screen.getByText('Nobody here is opted in, so nothing would go out.')).toBeDefined()
    unmount()
    reach({ total: null, eligible: null, split: null, pending: 'Pick a segment to see who gets it.' })
    expect(screen.getByText('Pick a segment to see who gets it.')).toBeDefined()
    expect(screen.queryByRole('img')).toBeNull()
  })
})

describe('the send date', () => {
  it('carries its text once for a screen reader, beside the rolled characters', () => {
    const { container } = render(<SendDate word="Not sent" beside={['Cancelled', 'before it went out']} />)
    expect(container.querySelector('.sr-only')?.textContent).toBe('Not sent')
    expect(container.querySelector('[aria-hidden]')?.textContent).toBe('Not sent')
    expect(screen.getByText('Cancelled')).toBeDefined()
  })
})
