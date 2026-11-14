import { useQuery } from '@apollo/client/react'
import type { MediaVisibility } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import { MediaPrivateAreaDocument } from './MediaLibrary.gql'

// Client-side mirror of the library's authz rules. The private half's grants sit on top of the library's and count only on all channels
export function useMediaAccess() {
  const { has, allowedChannelScopes, loading } = usePermissions()
  const everywhere = (slug: string) => allowedChannelScopes(slug).allChannels
  const mayOpen = has('media.view') && everywhere('media.private.view')
  const store = useQuery(MediaPrivateAreaDocument, { skip: !mayOpen })
  const hasPrivate = mayOpen && (store.data?.mediaPrivateArea ?? false)
  const managesPrivate = hasPrivate && has('media.manage') && everywhere('media.private.manage')
  return {
    hasPrivate,
    /** Whether it's known yet which halves this admin has - until then a page asked for the private one can't tell */
    settled: !loading && (!mayOpen || store.data !== undefined || store.error !== undefined),
    canManage: (area: MediaVisibility) => (area === 'PRIVATE' ? managesPrivate : has('media.manage')),
    canCross: managesPrivate,
    canShare: has('media.view') && everywhere('media.share'),
    canSeeCustomers: has('customers.view'),
  }
}
