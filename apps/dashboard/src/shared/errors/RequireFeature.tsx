import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { HomeIcon, PowerOffIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useFeatures } from '@/shared/features/useFeatures'
import { LoadError } from './LoadError'

const FEATURE_NAMES: Record<string, { name: string; env: string }> = {
  signals: { name: 'Signals', env: 'SIGNALS_ENABLED' },
  ai: { name: 'AI', env: 'AI_ENABLED' },
}

/** Renders children only while the server feature switch allows it - a '!' prefix inverts it, as on a nav leaf */
export function RequireFeature({ feature, children }: { feature: string; children: ReactNode }) {
  const { enabled, loading, known, error, refetch } = useFeatures()
  if (loading) return null
  if (!known) {
    return <LoadError title="Couldn't load the server's feature switches" message={error?.message} onRetry={() => void refetch()} />
  }
  const inverted = feature.startsWith('!')
  const flag = inverted ? feature.slice(1) : feature
  if (enabled(flag) !== inverted) return <>{children}</>
  return <FeatureOff feature={flag} inverted={inverted} />
}

function FeatureOff({ feature, inverted }: { feature: string; inverted: boolean }) {
  const known = FEATURE_NAMES[feature]
  const name = known?.name ?? feature
  return (
    <div className="flex min-h-[70vh] flex-col items-center justify-center gap-5 px-6 text-center">
      <div className="grid size-16 place-items-center rounded-full bg-background ring-1 ring-border shadow-sm">
        <PowerOffIcon className="size-8 text-muted-foreground" />
      </div>
      <div className="grid gap-1.5">
        <h1 className="text-2xl font-bold tracking-tight">{inverted ? `${name} is switched on` : `${name} is switched off`}</h1>
        <p className="max-w-md text-sm text-muted-foreground">
          {inverted
            ? `This page only applies while ${name} is disabled on this server.`
            : `This page belongs to ${name}, which is disabled on this server.`}
          {known && !inverted ? (
            <>
              {' '}Set{' '}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">{known.env}=true</code>{' '}
              to turn it on.
            </>
          ) : null}
        </p>
      </div>
      <Button asChild>
        <Link to="/">
          <HomeIcon className="size-4" />
          Dashboard
        </Link>
      </Button>
    </div>
  )
}
