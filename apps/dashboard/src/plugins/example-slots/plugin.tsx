// Example UI-slot plugin - a widget on the home page that core knows nothing about
// Location matters - loader.ts auto-imports src/plugins/<name>/plugin.{ts,tsx}, no core wiring

import { Badge } from '@/components/ui/badge'
import { DetailPageSection } from '@/components/layout/detail-page'
import { registerSlot } from '@/plugins/slot-registry'

registerSlot({
  id: 'example-slots:home-widget',
  zone: 'home:widgets',
  render: () => ( // its own section, so it sits in the divide-y with core's panels
    <DetailPageSection
      title="Example plugin widget"
      description="Registered from src/plugins/example-slots - core has no reference to it."
    >
      <div className="flex items-center gap-2 rounded-md border p-4 text-sm text-muted-foreground">
        <Badge variant="secondary">plugin</Badge>
        Anything a plugin renders here lands after the core panels, ordered by `order`.
      </div>
    </DetailPageSection>
  ),
})
