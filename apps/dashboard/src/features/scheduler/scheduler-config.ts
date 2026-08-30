import { useQuery } from '@apollo/client/react'
import { usePermissions } from '@/shared/auth/usePermissions'
import { SchedulerConfigDocument } from './Scheduler.gql'

// Split out of SchedulerSettings so that file exports only components - fast refresh needs the
// separation, and the scheduler's config scope is shared by both

export const DEFAULT_SCOPE = { kind: 'DEFAULT' as const }

export function useSchedulerPaused(): boolean {
  const { has } = usePermissions()
  const { data } = useQuery(SchedulerConfigDocument, {
    variables: { scope: DEFAULT_SCOPE },
    skip: !has('config.view'),
  })
  return data?.enabled?.value === 'false'
}
