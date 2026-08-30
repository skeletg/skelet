import { Rows2Icon, Rows3Icon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import { usePreference } from '@/lib/admin-prefs'
import {
  DEFAULT_PAGE_SIZE,
  DENSITY_PREF_KEY,
  PAGE_SIZE_PREF_KEY,
  type Density,
} from '@/components/data-table/useTablePrefs'

const OPTIONS: { value: Density; label: string; icon: typeof Rows2Icon }[] = [
  { value: 'comfortable', label: 'Comfortable', icon: Rows2Icon },
  { value: 'compact', label: 'Compact', icon: Rows3Icon },
]

// same steps the pagination footer offers, so a default is always one of the presets there
const SIZES = [10, 20, 50, 100]

export function DefaultsSection() {
  const [density, setDensity] = usePreference<Density>(DENSITY_PREF_KEY, 'comfortable')
  const [pageSize, setPageSize] = usePreference<number>(PAGE_SIZE_PREF_KEY, DEFAULT_PAGE_SIZE)

  return (
    <>
      <DetailPageSection
        title="Table density"
        description="How tightly list rows pack by default - tables you've set individually keep their own density until you reset them below."
      >
        <div className="flex gap-2">
          {OPTIONS.map((option) => (
            <Button
              key={option.value}
              variant="outline"
              aria-pressed={density === option.value}
              onClick={() => setDensity(option.value)}
              className={cn(density === option.value && 'border-primary bg-primary/10 text-primary')}
            >
              <option.icon className="size-4" />
              {option.label}
            </Button>
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Rows per page"
        description="How many rows a list shows before you change it in the footer. A link that names its own page size still wins."
      >
        <div className="flex gap-2">
          {SIZES.map((size) => (
            <Button
              key={size}
              variant="outline"
              aria-pressed={pageSize === size}
              // storing the default just deletes the key, so an untouched account keeps no row
              onClick={() => setPageSize(size === DEFAULT_PAGE_SIZE ? undefined : size)}
              className={cn('w-16', pageSize === size && 'border-primary bg-primary/10 text-primary')}
            >
              {size}
            </Button>
          ))}
        </div>
      </DetailPageSection>
    </>
  )
}
