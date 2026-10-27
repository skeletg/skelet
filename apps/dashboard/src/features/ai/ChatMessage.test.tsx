import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { stubResizeObserver } from '@/test/page-harness'
import { MessageContent } from './ChatMessage'

// What this pins: the chart widgets still render now that they load on demand (recharts left the signed-in shell)

describe('MessageContent chart widgets', () => {
  it('renders a bar widget once its chunk loads', async () => {
    stubResizeObserver()
    const content = 'Top sellers:\n```skelet-widget\n{"type":"bar","title":"Units by product","data":[{"label":"Tee","value":12}]}\n```'
    render(<MessageContent content={content} />)
    expect(screen.getByText('Top sellers:')).toBeTruthy()
    expect(await screen.findByText('Units by product', {}, { timeout: 5000 })).toBeTruthy()
  })

  it('renders a line widget once its chunk loads', async () => {
    stubResizeObserver()
    const content = '{"type":"line","title":"Daily orders","series":[{"label":"Orders","data":[{"x":"Mon","y":3},{"x":"Tue","y":5}]}]}'
    render(<MessageContent content={content} />)
    expect(await screen.findByText('Daily orders', {}, { timeout: 5000 })).toBeTruthy()
  })
})
