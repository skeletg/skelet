import type { ReactNode } from 'react'
import { screen } from '@testing-library/react'
import type { DocumentNode } from 'graphql'
import { describe, expect, it } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { ConsentHistorySection } from './ConsentHistorySection'
import { CustomerConsentHistoryDocument, LoyaltyDocument, StoreCreditDocument } from './Customers.gql'
import { LoyaltySection } from './LoyaltySection'
import { StoreCreditSection } from './StoreCreditSection'

// What these pin: a customer section whose load fails says so - store credit and loyalty returned null on an error,
// so the section vanished as if the customer had none, and consent history read as "no changes"

const sections: Array<[string, ReactNode, DocumentNode]> = [
  ['StoreCreditSection', <StoreCreditSection customerId="cu1" />, StoreCreditDocument],
  ['LoyaltySection', <LoyaltySection customerId="cu1" />, LoyaltyDocument],
  ['ConsentHistorySection', <ConsentHistorySection customerId="cu1" kindName={(k) => k} />, CustomerConsentHistoryDocument],
]

describe('customer sections on a failed load', () => {
  it.each(sections)('%s shows the error', async (_name, section, query) => {
    renderPage({
      path: '/x',
      routes: { '/x': section },
      adminId: 'admin-1',
      mocks: [meMock(), { request: { query, variables: { customerId: 'cu1' } }, error: new Error('customers are down') }],
    })
    expect(await screen.findByText('customers are down', {}, SLOW)).toBeTruthy()
  })
})
