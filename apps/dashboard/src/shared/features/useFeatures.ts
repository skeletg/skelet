import { useQuery } from '@apollo/client/react'
import { ServerFeaturesDocument } from './Features.gql'

// Server-wide feature switches, env-driven on the backend - cache-first like usePermissions, so
// one fetch per session, and `enabled()` is false while loading so gated nav stays hidden
export function useFeatures() {
  const { data, loading, error, refetch } = useQuery(ServerFeaturesDocument, { fetchPolicy: 'cache-first' })
  const features = data?.serverFeatures

  return {
    loading,
    // False until the switches have actually been read, so a failed fetch never reads as "switched off"
    known: features != null,
    error,
    refetch,
    enabled: (flag: string): boolean => {
      if (!features) return false
      switch (flag) {
        case 'signals':
          return features.signals
        case 'ai':
          return features.ai
        default:
          return true
      }
    },
  }
}
