import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ResponseDetailsBar } from './ResponseDetailsBar'
import type { ResponseDetails } from './responseDetails'

const details: ResponseDetails = {
  status: 200,
  statusText: 'OK',
  durationMs: 142,
  sizeBytes: 3482,
  errorCount: 0,
  dbQueries: 7,
  traceId: 'abc123',
  requestId: 'req-1',
}

describe('ResponseDetailsBar', () => {
  it('prompts before the first run', () => {
    render(<ResponseDetailsBar details={null} />)
    expect(screen.getByText(/run an operation/i)).toBeTruthy()
  })

  it('shows status, time, size, query count and a trace link', () => {
    render(<ResponseDetailsBar details={details} />)
    expect(screen.getByText('200 OK')).toBeTruthy()
    expect(screen.getByText('142 ms')).toBeTruthy()
    expect(screen.getByText('3.4 KB')).toBeTruthy()
    expect(screen.getByText('7')).toBeTruthy()
    expect(screen.getByRole('link', { name: /trace/i }).getAttribute('href')).toMatch(/\/trace\/abc123$/)
    expect(screen.queryByText('Errors')).toBeNull()
  })

  it('counts errors and drops the dev-only fields when the headers are absent', () => {
    render(<ResponseDetailsBar details={{ ...details, errorCount: 2, dbQueries: null, traceId: null }} />)
    expect(screen.getByText('Errors')).toBeTruthy()
    expect(screen.getByText('2')).toBeTruthy()
    expect(screen.queryByText('DB queries')).toBeNull()
    expect(screen.queryByRole('link', { name: /trace/i })).toBeNull()
  })
})
