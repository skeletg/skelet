import { Fragment } from 'react'
import { getDetailSections, type DetailSectionPlacement } from './detail-section-registry'

// Only the components live here, so their importers keep this path - the registry the plugins
// call at import time is in detail-section-registry, which fast refresh needs

// 'before' goes first in the page body, 'after' (the default) last - and OUTSIDE any wrapping <form>, so plugin Enter
// handlers can't trip the core submit. 'after' also carries the side sections on screens too narrow for the side
// panel, which hides there, so a side section is never lost
export function PluginSections<TEntity>({
  target,
  entity,
  mode,
  placement = 'after',
  sideInBody = false,
}: {
  target: string
  entity: TEntity
  mode: 'edit' | 'create'
  placement?: DetailSectionPlacement
  // For a page whose side panel is something else entirely, a live preview - the side sections then stay in the body
  sideInBody?: boolean
}) {
  const list = getDetailSections(target, mode, placement)
  const narrow = placement === 'after' ? getDetailSections(target, mode, 'side') : []
  if (list.length === 0 && narrow.length === 0) return null
  return (
    <>
      {list.map((s) => (
        <Fragment key={s.id}>{s.render({ entity, mode, placement })}</Fragment>
      ))}
      {narrow.map((s) => (
        <div key={s.id} className={sideInBody ? undefined : 'xl:hidden'}>
          {s.render({ entity, mode, placement: 'side' })}
        </div>
      ))}
    </>
  )
}

// A page's side panel holding only plugin sections, for a page without a panel of its own - nothing when none are
// registered, and the layout's main column then fills the width as it does with no panel. A page with a panel of its
// own renders <PluginSections placement="side"> inside it instead
export function PluginAside<TEntity>({ target, entity, mode }: { target: string; entity: TEntity; mode: 'edit' | 'create' }) {
  if (getDetailSections(target, mode, 'side').length === 0) return null
  return (
    <aside className="scrollbar-accent hidden h-full w-[20rem] shrink-0 flex-col gap-4 overflow-y-auto border-l bg-muted/10 p-4 xl:flex 2xl:w-[22rem]">
      <PluginSections target={target} entity={entity} mode={mode} placement="side" />
    </aside>
  )
}
