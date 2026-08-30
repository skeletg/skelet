import { usePermissions } from '@/shared/auth/usePermissions'

// Client-side mirror of the customers authz rules - per-customer mutations are channel-scoped, but group CRUD needs a GLOBAL
// customers.manage grant, which usePermissions surfaces as the allChannels flag (a scope row with no channel and no locale)
export function useCustomerAccess() {
  const { has, allowedChannelScopes } = usePermissions()
  return {
    canManage: has('customers.manage'),
    canManageGroups: allowedChannelScopes('customers.manage').allChannels,
  }
}
