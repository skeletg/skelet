import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { SignalsCustomerNamesDocument } from './CustomerNames.gql'

export type CustomerName = { name: string; email: string }

// Labels for the customer ids a Signals screen is showing, fetched from core in one batched follow-up
// Pass a memoized array - it's the query variable, so a fresh one every render refetches
export function customerLabel(found: CustomerName | undefined, customerID: string): string {
  return found?.name || found?.email || customerID.slice(0, 8)
}

export function useCustomerNames(ids: string[]): Map<string, CustomerName> {
  const { data } = useQuery(SignalsCustomerNamesDocument, {
    variables: { ids },
    skip: ids.length === 0,
    fetchPolicy: 'cache-first',
  })
  return useMemo(() => {
    const byID = new Map<string, CustomerName>()
    for (const c of data?.customersByIds ?? []) {
      byID.set(c.id, { name: [c.firstName, c.lastName].filter(Boolean).join(' ').trim(), email: c.email })
    }
    return byID
  }, [data])
}
