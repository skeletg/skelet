import { Slot } from '@/plugins/slots'

// Plugins render their own sections, so this one deliberately has no heading of its own
export function PluginWidget() {
  return <Slot zone="home:widgets" />
}
