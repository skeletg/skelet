import type { State } from './site'

// A page's state as a colour: the state tokens, never a palette hue
export const STATE_COLOR: Record<State, string> = {
  live: 'bg-success',
  scheduled: 'bg-warning',
  review: 'bg-info',
  approved: 'bg-info/45',
  draft: 'bg-muted-foreground/35',
}
