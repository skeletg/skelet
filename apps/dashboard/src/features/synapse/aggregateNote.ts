import { formatNumber } from '@/lib/format'

// Visitors who hadn't allowed analytics are counted with no identity - in event totals, never as people - so a total
// says how many of its events they are, and a count of people has nothing to say about them
export function aggregateNote(measure: string, aggregate: number): string | null {
  if (measure === 'unique' || aggregate <= 0) return null
  return `includes ${formatNumber(aggregate)} from visitors who declined analytics`
}
