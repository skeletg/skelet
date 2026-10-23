import { useEffect, useState } from 'react'
import { usePrefsSettled } from '@/lib/admin-prefs'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'

const SHELL_READY_TIMEOUT_MS = 3000

// Grants, switches and the saved arrangement are all in - or the timeout passed, so a failed prefs fetch can't hold the skeletons
export function useShellReady(): boolean {
  const { loading: permsLoading } = usePermissions()
  const { loading: featuresLoading } = useFeatures()
  const prefsSettled = usePrefsSettled()
  const [timedOut, setTimedOut] = useState(false)
  const [latched, setLatched] = useState(false)

  useEffect(() => {
    const timer = setTimeout(() => setTimedOut(true), SHELL_READY_TIMEOUT_MS)
    return () => clearTimeout(timer)
  }, [])

  const ready = latched || timedOut || (!permsLoading && !featuresLoading && prefsSettled)
  if (ready && !latched) setLatched(true)
  return ready
}
