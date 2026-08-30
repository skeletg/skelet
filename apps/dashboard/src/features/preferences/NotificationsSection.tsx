import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import { cn } from '@/lib/utils'
import {
  NOTIFICATION_CATEGORIES,
  TOAST_THRESHOLDS,
  useMutedCategories,
  useToastMinSeverity,
} from '@/features/notifications/preferences'

function Categories() {
  const { muted, isMuted, toggle, unmuteAll } = useMutedCategories()

  return (
    <div className="max-w-3xl space-y-2">
      <div className="divide-y rounded-md border">
        {NOTIFICATION_CATEGORIES.map((category) => {
          const off = isMuted(category.id)
          return (
            <label key={category.id} className="flex cursor-pointer items-center gap-3 px-3 py-2">
              <span
                className={cn('h-8 w-0.5 shrink-0 rounded-full', off ? 'bg-transparent' : 'bg-primary/40')}
              />
              <span className="min-w-0 flex-1">
                <span className={cn('block text-sm', off && 'text-muted-foreground')}>{category.label}</span>
                <span className="block text-xs text-muted-foreground">{category.hint}</span>
              </span>
              <Switch checked={!off} onCheckedChange={() => toggle(category.id)} />
            </label>
          )
        })}
      </div>
      {muted.length > 0 && (
        <Button variant="ghost" size="sm" onClick={unmuteAll}>
          Turn everything back on
        </Button>
      )}
    </div>
  )
}

function ToastThreshold() {
  const { min, setMin } = useToastMinSeverity()

  return (
    <div className="flex flex-wrap gap-2">
      {TOAST_THRESHOLDS.map((t) => (
        <Button
          key={t.id}
          variant="outline"
          aria-pressed={min === t.id}
          onClick={() => setMin(t.id)}
          title={t.hint}
          className={cn(min === t.id && 'border-primary bg-primary/10 text-primary')}
        >
          {t.label}
        </Button>
      ))}
    </div>
  )
}

export function NotificationsSection() {
  return (
    <>
      <DetailPageSection
        title="Notifications"
        description="A muted category leaves your inbox and your unread badge entirely - turn it back on and its history reappears."
      >
        <Categories />
      </DetailPageSection>

      <DetailPageSection
        title="Pop-up alerts"
        description="How much of it interrupts you as it happens. Everything still lands in the inbox either way."
      >
        <ToastThreshold />
      </DetailPageSection>
    </>
  )
}
