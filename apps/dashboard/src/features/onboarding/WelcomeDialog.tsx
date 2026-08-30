import { useEffect, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'
import { useAuth } from '@/shared/auth/auth'
import { WELCOME_STEPS, type WelcomeStep } from './steps'
import { Spotlight } from './Spotlight'
import { clearFirstLogin, peekFirstLogin } from './first-login'
import { WelcomeContext, visibleSteps } from './welcome-context'

export function WelcomeProvider({ children }: { children: ReactNode }) {
  const { auth } = useAuth()
  const { has, loading: permsLoading } = usePermissions()
  const { enabled: featureEnabled, loading: featuresLoading } = useFeatures()
  // The login form leaves a marker; this reads it once at mount (a pure read) and the effect below
  // clears it, so a reload can't restart the tour after it has been seen
  const [pending, setPending] = useState(peekFirstLogin)
  const [manual, setManual] = useState(false)

  useEffect(() => {
    if (pending) clearFirstLogin()
  }, [pending])

  // Held until the grants are in, or the first step lists nothing
  const ready = !!auth && !permsLoading && !featuresLoading
  const open = manual || (pending && ready)

  const close = () => {
    setManual(false)
    setPending(false)
  }

  return (
    <WelcomeContext.Provider value={{ open: () => setManual(true) }}>
      {children}
      {open && (
        <WelcomeTour steps={visibleSteps(WELCOME_STEPS, { has, featureEnabled })} onClose={close} />
      )}
    </WelcomeContext.Provider>
  )
}

function WelcomeTour({ steps, onClose }: { steps: WelcomeStep[]; onClose: () => void }) {
  const [index, setIndex] = useState(0)
  if (steps.length === 0) return null

  const step = steps[Math.min(index, steps.length - 1)]
  const last = index >= steps.length - 1

  return (
    <Spotlight selector={step.target} onSkip={onClose}>
      <div className="grid gap-3">
        <div className="grid gap-1">
          <h2 className="text-base font-semibold">{step.title}</h2>
          <step.Body />
        </div>

        <div className="flex items-center justify-between gap-3 pt-1">
          <div className="flex items-center gap-1.5">
            {steps.map((s, i) => (
              <span
                key={s.id}
                aria-hidden="true"
                className={cn(
                  'h-1.5 rounded-full transition-all',
                  i === index ? 'w-4 bg-primary' : 'w-1.5 bg-muted-foreground/30',
                )}
              />
            ))}
          </div>
          <div className="flex items-center gap-1">
            {index > 0 && (
              <Button variant="ghost" size="sm" onClick={() => setIndex((i) => i - 1)}>
                Back
              </Button>
            )}
            {!last && (
              <Button variant="ghost" size="sm" onClick={onClose}>
                Skip
              </Button>
            )}
            {/* autoFocus so Enter walks the tour and the card owns the keyboard from the start */}
            <Button size="sm" autoFocus onClick={() => (last ? onClose() : setIndex((i) => i + 1))}>
              {last ? 'Done' : 'Next'}
            </Button>
          </div>
        </div>
      </div>
    </Spotlight>
  )
}
