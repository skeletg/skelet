import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderDialog } from '@/test/page-harness'
import { PageBlockCanvas } from './PageBlockCanvas'

// What this pins: a block opens on its content fields, with tracking and visibility folded under Advanced -
// and a collapsed Advanced still names what is set inside it

describe('PageBlockCanvas', () => {
  it('folds tracking and visibility under Advanced, naming what is set', () => {
    renderDialog(
      <PageBlockCanvas blocks={[{ id: 'b1', type: 'cta_banner', props: { heading: 'New here?', goalEvent: 'cta_click' } }]} onChange={() => {}} />,
    )
    fireEvent.click(screen.getByRole('button', { name: /CTA banner/ }))
    expect(screen.getByText('Heading')).toBeDefined()
    expect(screen.queryByText('A/B goal event')).toBeNull()
    expect(screen.queryByText('Visibility')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /^Advanced.*A\/B goal event set$/ }))
    expect(screen.getByText('A/B goal event')).toBeDefined()
    expect(screen.getByText('Visibility')).toBeDefined()

    // No animation API here, so a closed panel goes at once rather than waiting on a finish that never comes
    fireEvent.click(screen.getByRole('button', { name: /CTA banner/ }))
    expect(screen.queryByText('Heading')).toBeNull()
  })
})
