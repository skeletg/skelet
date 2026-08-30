import { Fragment } from 'react'
import { getDetailSections } from './detail-section-registry'

// Only the component lives here, so its ~27 importers keep this path - the registry the plugins
// call at import time moved to detail-section-registry, which fast refresh needs

export function PluginSections<TEntity>({  // place after the core sections and OUTSIDE any wrapping <form>, so plugin Enter handlers can't trip the core submit
  target,
  entity,
  mode,
}: {
  target: string
  entity: TEntity
  mode: 'edit' | 'create'
}) {
  const list = getDetailSections(target, mode)
  if (list.length === 0) return null
  return (
    <>
      {list.map((s) => (
        <Fragment key={s.id}>{s.render({ entity, mode })}</Fragment>
      ))}
    </>
  )
}
