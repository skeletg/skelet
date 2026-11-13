import { cn } from '@/lib/utils'
import type { State } from './site'
import { STATE_COLOR } from './state-colors'

export function Mark({ state }: { state: State }) {
  return <span aria-hidden className={cn('inline-block size-2 shrink-0 rounded-[2px]', STATE_COLOR[state])} />
}
