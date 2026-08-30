// Loads currencies once at boot into lib/format.ts's registry, so every price callsite honours the backend's per-currency symbolPosition without plumbing a Currency object through

import { useQuery } from '@apollo/client/react'
import { useEffect, type ReactNode } from 'react'

import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { setCurrencyRegistry } from '@/lib/format'

export function CurrencyRegistryProvider({ children }: { children: ReactNode }) { // mounted inside the auth-protected branch, so the query only runs for signed-in admins; until it resolves formatCents falls back to Intl
  const { data } = useQuery(CurrenciesPickerDocument)
  useEffect(() => {
    if (data?.currencies) {
      setCurrencyRegistry(data.currencies)
    }
  }, [data])
  return <>{children}</>
}
