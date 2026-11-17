import { useSyncExternalStore } from 'react'
import { getDetailSections, type DetailSectionPlacement } from './detail-section-registry'
import { PluginRender } from './PluginBoundary'

// Only the components live here, so their importers keep this path - the registry the plugins
// call at import time is in detail-section-registry, which fast refresh needs

// Tailwind's xl, where a detail page's side panel shows
const WIDE = '(min-width: 80rem)'

// A side section is mounted once: in the panel on a wide screen, in the body below that. Hidden with CSS instead, both
// copies ran their queries, and a section staging an edit staged it twice
function useWide(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const mql = window.matchMedia?.(WIDE)
      mql?.addEventListener('change', onChange)
      return () => mql?.removeEventListener('change', onChange)
    },
    () => window.matchMedia?.(WIDE).matches ?? true,
  )
}

// 'before' goes first in the page body, 'after' (the default) last - and OUTSIDE any wrapping <form>, so plugin Enter
// handlers can't trip the core submit. 'after' also carries the side sections on screens too narrow for the side
// panel, so a side section is never lost; 'side' draws them only where that panel shows
export function PluginSections<TEntity>({
  target,
  entity,
  mode,
  placement = 'after',
  sideInBody = false,
  inPanel = false,
}: {
  target: string
  entity: TEntity
  mode: 'edit' | 'create'
  placement?: DetailSectionPlacement
  // For a page whose side panel is something else entirely, a live preview, or is folded away - the side sections
  // then stay in the body
  sideInBody?: boolean
  // For a page that draws every section in its panel: each is told it's in one, whatever placement it asked for
  inPanel?: boolean
}) {
  const wide = useWide()
  const list = placement === 'side' && !wide ? [] : getDetailSections(target, mode, placement)
  const carried = placement === 'after' && (sideInBody || !wide) ? getDetailSections(target, mode, 'side') : []
  if (list.length === 0 && carried.length === 0) return null
  return (
    <>
      {list.map((s) => (
        <PluginRender key={s.id} id={s.id} render={() => s.render({ entity, mode, placement: inPanel ? 'side' : placement })} />
      ))}
      {carried.map((s) => (
        <PluginRender key={s.id} id={s.id} render={() => s.render({ entity, mode, placement: 'side' })} />
      ))}
    </>
  )
}

// A page's side panel holding only plugin sections, for a page without a panel of its own - nothing when none are
// registered or the screen is too narrow for it, and the layout's main column then fills the width as it does with no
// panel. A page with a panel of its own renders <PluginSections placement="side"> inside it instead
export function PluginAside<TEntity>({ target, entity, mode }: { target: string; entity: TEntity; mode: 'edit' | 'create' }) {
  const wide = useWide()
  if (!wide || getDetailSections(target, mode, 'side').length === 0) return null
  return (
    <aside className="scrollbar-accent flex h-full w-[20rem] shrink-0 flex-col gap-4 overflow-y-auto border-l bg-muted/10 p-4 2xl:w-[22rem]">
      <PluginSections target={target} entity={entity} mode={mode} placement="side" />
    </aside>
  )
}
