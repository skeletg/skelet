import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { CartStatusBadges } from './CartStatusBadges'

describe('CartStatusBadges', () => {
  it('names a handed-off agent cart in words and marks where it came from', () => {
    render(<CartStatusBadges status="handed_off" source="agent" />)
    expect(screen.getByText('Handed off')).toBeTruthy()
    expect(screen.getByText('Agent')).toBeTruthy()
  })

  it("shows a shopper's own cart without the agent mark, and an unknown status as it is", () => {
    render(<CartStatusBadges status="frozen" source="web" />)
    expect(screen.getByText('frozen')).toBeTruthy()
    expect(screen.queryByText('Agent')).toBeNull()
  })
})
