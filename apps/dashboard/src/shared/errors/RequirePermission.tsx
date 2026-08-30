import type { ReactNode } from 'react'
import { usePermissions } from '@/shared/auth/usePermissions'
import { Forbidden } from './Forbidden'

/** Renders children only for an admin holding the slug - nothing while `me` loads, so the Forbidden screen can't flash, and superusers always pass */
export function RequirePermission({
  permission,
  children,
}: {
  permission: string
  children: ReactNode
}) {
  const { has, loading } = usePermissions()
  if (loading) return null
  if (!has(permission)) return <Forbidden permission={permission} />
  return <>{children}</>
}
