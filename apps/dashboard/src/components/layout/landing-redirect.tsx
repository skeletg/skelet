import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { usePrefsLoaded } from '@/lib/admin-prefs'
import { DEFAULT_LANDING, useLandingPref } from '@/lib/landing'

// Sends the admin to their preferred landing page instead of the dashboard - it bounces from
// inside the shell because the preference store only exists behind auth. Deep links are
// unaffected: ProtectedRoute remembers where you were headed, so a shared link never lands on '/'.
// The ref keeps it to once per load, so clicking Dashboard later actually goes to the dashboard
export function LandingRedirect() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const loaded = usePrefsLoaded()
  const { landing, resolving } = useLandingPref()
  const done = useRef(false)

  useEffect(() => {
    if (done.current) return
    if (!loaded || resolving) return // acting early would race the roaming copy or the nav gating
    done.current = true
    if (pathname === DEFAULT_LANDING && landing !== DEFAULT_LANDING) {
      navigate(landing, { replace: true })
    }
  }, [loaded, resolving, pathname, landing, navigate])

  return null
}
