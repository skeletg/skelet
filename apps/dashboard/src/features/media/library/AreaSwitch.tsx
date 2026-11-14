import { GlobeIcon, LockIcon } from 'lucide-react'
import type { MediaVisibility } from '@/graphql/schema.types'
import { cn } from '@/lib/utils'

const AREAS: { value: MediaVisibility; label: string; about: string; icon: typeof LockIcon }[] = [
  { value: 'PUBLIC', label: 'Public', about: 'Files with an address anyone can open - what the storefront shows', icon: GlobeIcon },
  { value: 'PRIVATE', label: 'Private', about: 'Files with no address - only admins, and customers they are shared with, can open them', icon: LockIcon },
]

// The library's two halves. They keep separate folders, so switching is a change of place, not a filter
export function AreaSwitch({ area, onArea, className }: { area: MediaVisibility; onArea: (area: MediaVisibility) => void; className?: string }) {
  return (
    <div role="group" aria-label="Which files" className={cn('flex rounded-full border p-0.5 text-xs', className)}>
      {AREAS.map((a) => (
        <button
          key={a.value}
          type="button"
          aria-pressed={area === a.value}
          title={a.about}
          onClick={() => area !== a.value && onArea(a.value)}
          className={cn(
            'flex flex-1 items-center justify-center gap-1.5 rounded-full px-3 py-1 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
            area === a.value ? 'bg-foreground text-background' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          <a.icon className="size-3.5" />
          {a.label}
        </button>
      ))}
    </div>
  )
}
