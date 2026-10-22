import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'

import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { useConfigScope } from '@/lib/config-scope'

// Per-channel store timezone, default "UTC" - no single-value read exists, so this reuses the
// Settings config query: cache-first, and fail-soft so an admin without config-read just sees no clock
const TIMEZONE_PATH = 'general/store_information/timezone'

// Live clock in the in-scope store's timezone - renders nothing when it's unset, not a valid
// IANA zone, or the config read isn't permitted
export function StoreClock() {
  const { scopeInput } = useConfigScope()
  const { data } = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    errorPolicy: 'all',
    fetchPolicy: 'cache-first',
  })

  const timezone = useMemo(() => {
    const fields = data?.configSections.flatMap((s) => s.groups.flatMap((g) => g.fields)) ?? []
    return fields.find((f) => f.path === TIMEZONE_PATH)?.value ?? ''
  }, [data])

  // One formatter per timezone - recreating it each tick is wasteful, and an invalid zone throws here, caught once rather than every second
  const formatter = useMemo(() => {
    if (!timezone) return null
    try {
      return new Intl.DateTimeFormat(undefined, {
        timeZone: timezone,
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    } catch {
      return null
    }
  }, [timezone])

  // The clock shows minutes, so it ticks on each minute boundary - every zone's offset is whole minutes - instead of
  // re-rendering the sidebar every second
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const untilNextMinute = () => 60_000 - (Date.now() % 60_000)
    let id = setTimeout(function tick() {
      setNow(new Date())
      id = setTimeout(tick, untilNextMinute())
    }, untilNextMinute())
    return () => clearTimeout(id)
  }, [])

  if (!formatter) return null
  return (
    // A labeled row shaped for the sidebar's scope card, where the clock lives under the pickers
    <div className="mt-3 flex items-baseline justify-between gap-2 border-t pt-2.5">
      <span className="text-xs text-muted-foreground">Channel time</span>
      <span className="text-xs whitespace-nowrap tabular-nums" title={timezone}>
        {formatter.format(now)}
      </span>
    </div>
  )
}
