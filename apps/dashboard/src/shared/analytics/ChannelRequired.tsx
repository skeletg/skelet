import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'

// ChannelRequired is a report's empty state on global scope - it offers the shops right here rather than sending the
// admin to find the scope picker
export function ChannelRequired({ what }: { what: string }) {
  const { channels, setScope } = useConfigScope()
  return (
    <Alert>
      <AlertTitle>Pick a shop</AlertTitle>
      <AlertDescription>
        <p>{what} are per shop.</p>
        {channels.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-2">
            {channels.map((c) => (
              <Button key={c.id} size="sm" variant="outline" onClick={() => setScope({ mode: 'CHANNEL', channelId: c.id, locale: '' })}>
                {c.name}
              </Button>
            ))}
          </div>
        )}
      </AlertDescription>
    </Alert>
  )
}
