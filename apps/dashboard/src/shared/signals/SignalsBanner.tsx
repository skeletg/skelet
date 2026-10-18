import { useSyncExternalStore } from 'react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { signalsIsDown, subscribeSignalsHealth } from './health'

// Mounted once in the shell - a Signals screen that cannot reach its service would otherwise look like a screen with no data
export function SignalsBanner() {
  const down = useSyncExternalStore(subscribeSignalsHealth, signalsIsDown, () => false)
  if (!down) return null
  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-0 z-50 flex justify-center p-4">
      <Alert variant="destructive" className="pointer-events-auto max-w-xl shadow-lg">
        <AlertTitle>Signals is not reachable</AlertTitle>
        <AlertDescription>
          Analytics screens are served by the Signals service, which is not answering, so they will look empty. Its
          data is unaffected. In development, start it with{' '}
          <code className="font-mono text-xs">docker compose --profile signals up -d signals</code>.
        </AlertDescription>
      </Alert>
    </div>
  )
}
