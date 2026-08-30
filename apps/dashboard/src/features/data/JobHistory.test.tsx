import { render, screen, waitFor } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import { describe, expect, it } from 'vitest'

import { DataJobsDocument } from './Data.gql'
import { JobHistory } from './JobHistory'

function job(overrides: Record<string, unknown> = {}) {
  return {
    __typename: 'DataJob',
    id: 'job-1',
    direction: 'export',
    resource: 'products',
    resourceLabel: 'Products',
    format: 'csv',
    status: 'succeeded',
    dryRun: false,
    channelId: null,
    filename: 'products.csv',
    hasArtifact: true,
    byteSize: 2048,
    totalRows: 10,
    processedRows: 10,
    createdRows: 0,
    updatedRows: 10,
    skippedRows: 0,
    failedRows: 0,
    errorSummary: '',
    warnings: [],
    errorsTruncated: false,
    createdAt: new Date().toISOString(),
    startedAt: null,
    finishedAt: null,
    expiresAt: null,
    ...overrides,
  }
}

function result(items: ReturnType<typeof job>[]) {
  return { data: { dataJobs: { __typename: 'DataJobList', totalCount: items.length, items } } }
}

const request = {
  query: DataJobsDocument,
  variables: { resources: ['products'], limit: 25 },
}

describe('JobHistory', () => {
  it('shows the runs it fetched', async () => {
    render(
      <MockedProvider mocks={[{ request, result: result([job()]) }]}>
        <JobHistory resourceSlug="products" />
      </MockedProvider>,
    )
    expect(await screen.findByText('Products')).toBeTruthy()
    expect(screen.getByText(/10 rows/)).toBeTruthy() // an export reports rows out, not created/updated
  })

  // The regression this exists for: a freshly queued job was invisible until a reload, because the query only
  // fetches on mount and only starts polling once it has ALREADY seen something running - which it never could
  it('refetches when the page signals a run was queued', async () => {
    const mocks = [
      { request, result: result([]) },
      { request, result: result([job({ status: 'queued' })]) },
    ]
    const { rerender } = render(
      <MockedProvider mocks={mocks}>
        <JobHistory resourceSlug="products" reloadKey={0} />
      </MockedProvider>,
    )
    expect(await screen.findByText(/Nothing has been imported or exported yet/)).toBeTruthy()

    rerender(
      <MockedProvider mocks={mocks}>
        <JobHistory resourceSlug="products" reloadKey={1} />
      </MockedProvider>,
    )
    await waitFor(() => expect(screen.queryByText('Products')).toBeTruthy())
  })

  it('offers cancel while a run is still going, not just before it starts', async () => {
    render(
      <MockedProvider mocks={[{ request, result: result([job({ status: 'running' })]) }]}>
        <JobHistory resourceSlug="products" />
      </MockedProvider>,
    )
    expect(await screen.findByText('Products')).toBeTruthy()
    expect(screen.getByRole('button', { name: /cancel/i })).toBeTruthy()
  })

  it('links to the failed rows when an import had any', async () => {
    render(
      <MockedProvider
        mocks={[
          { request, result: result([job({ direction: 'import', status: 'failed', failedRows: 3 })]) },
        ]}
      >
        <JobHistory resourceSlug="products" />
      </MockedProvider>,
    )
    expect(await screen.findByRole('button', { name: /3 failed rows/i })).toBeTruthy()
  })
})
