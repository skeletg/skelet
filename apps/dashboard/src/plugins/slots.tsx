import { getSlots } from './slot-registry'
import { PluginRender } from './PluginBoundary'

// Only the component lives here - the registry is in slot-registry so fast refresh works on both

export function Slot<TCtx = unknown>({ zone, ctx }: { zone: string | undefined; ctx?: TCtx }) {
  const entries = getSlots<TCtx>(zone)
  if (entries.length === 0) return null // nothing at all, so a host can drop it into a flex row unconditionally
  return (
    <>
      {entries.map((s) => (
        <PluginRender key={s.id} id={s.id} render={() => s.render(ctx as TCtx)} />
      ))}
    </>
  )
}
