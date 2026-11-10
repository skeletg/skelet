import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { ReplyText } from './ReplyText'

describe('ReplyText', () => {
  it('renders the light markdown the assistant writes, the way the storefront showed it', () => {
    const { container } = render(
      <ReplyText
        text={'## Two options\nNo to both, **I\'m afraid**.\n\n- **Unicorn saddles:** none in the catalog\n- Use `SPRING10` at checkout\n\n1. Pick a size\n2. Add to cart'}
      />,
    )
    expect(screen.getByText('Two options')).toHaveClass('font-semibold')
    expect(screen.getByText("I'm afraid").tagName).toBe('STRONG')
    expect(screen.getByText('Unicorn saddles:').tagName).toBe('STRONG')
    expect(screen.getByText('SPRING10').tagName).toBe('CODE')
    expect(container.querySelectorAll('ul > li')).toHaveLength(2)
    expect(container.querySelectorAll('ol > li')).toHaveLength(2)
    expect(container.textContent).not.toMatch(/\*\*|##|`/)
  })

  it('shows where a link pointed without making it clickable', () => {
    const { container } = render(<ReplyText text={'See [our returns page](https://shop.example/returns) or https://evil.example/x.'} />)
    expect(container.querySelector('a')).toBeNull()
    expect(screen.getByText('our returns page')).toBeInTheDocument()
    expect(screen.getByText('https://shop.example/returns')).toBeInTheDocument()
    expect(screen.getByText('https://evil.example/x')).toBeInTheDocument()
    expect(container.textContent).toMatch(/evil\.example\/x\.$/)
  })

  it('numbers a list, and reads any heading level as one bold line', () => {
    const { container } = render(<ReplyText text={'# Big\n###### Small\n1. First **pick**\n2) Second\n10. Tenth'} />)
    expect(screen.getByText('Big')).toHaveClass('font-semibold')
    expect(screen.getByText('Small')).toHaveClass('font-semibold')
    expect([...container.querySelectorAll('ol > li')].map((li) => li.textContent)).toEqual(['First pick', 'Second', 'Tenth'])
    expect(screen.getByText('pick').tagName).toBe('STRONG')
    expect(container.textContent).not.toMatch(/#|\d[.)]/)
  })

  it('keeps punctuation after a bare address out of the address', () => {
    const { container } = render(<ReplyText text={'Try https://shop.example/a?b=1, or https://shop.example/c! Then "https://shop.example/d".'} />)
    expect(screen.getByText('https://shop.example/a?b=1')).toBeInTheDocument()
    expect(screen.getByText('https://shop.example/c')).toBeInTheDocument()
    expect(screen.getByText('https://shop.example/d')).toBeInTheDocument()
    expect(container.textContent).toBe('Try https://shop.example/a?b=1, or https://shop.example/c! Then "https://shop.example/d".')
  })

  it('leaves a script link as plain text', () => {
    const { container } = render(<ReplyText text={'Click [here](javascript:alert(1)) for a deal'} />)
    expect(container.querySelector('a')).toBeNull()
    expect(container.querySelector('[href]')).toBeNull()
    expect(screen.getByText('here')).toBeInTheDocument()
    expect(container.textContent).toContain('javascript:alert(1')
  })
})
