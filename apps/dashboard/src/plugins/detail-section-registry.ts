import type { ReactNode } from 'react'

// Detail-page section registry - plugins call registerDetailSection at import time and each
// entry renders in the matching entity's page via <PluginSections>: at the top of its body, at the
// end of it (the default), or in its side panel
// A section owns its own state, mutations and save UX - wiring into the parent form's Save
// button is out of scope, use useSaveSection (shared/saving/saveRegistry) for that

export type DetailSectionMode = 'edit' | 'create' | 'both'

// 'side' is the panel beside the page on wide screens - below that the panel is hidden, so a side
// section renders at the end of the body instead
export type DetailSectionPlacement = 'before' | 'after' | 'side'

export type DetailSectionEntry<TEntity = unknown> = {
  id: string  // slug-style, conventionally '<plugin-name>:section' - duplicates throw at registration
  target: string  // free-form, so a new detail page needs no core registry change
  order?: number  // lower runs earlier, default 100 - core sections sit at 10-50
  mode?: DetailSectionMode  // default 'edit', since most plugin sections need an existing entity id
  placement?: DetailSectionPlacement  // default 'after'
  // In the body it must produce a <DetailPageSection> at its root so the surrounding divide-y works - in the side
  // panel a plain block with a small heading, like the panel's own
  render: (props: { entity: TEntity; mode: 'edit' | 'create'; placement: DetailSectionPlacement }) => ReactNode
}

const sections: DetailSectionEntry[] = []

export function registerDetailSection<TEntity = unknown>(entry: DetailSectionEntry<TEntity>) {
  if (sections.some((s) => s.id === entry.id)) {
    throw new Error(`detail-sections: duplicate id "${entry.id}"`)
  }
  sections.push(entry as DetailSectionEntry)
}

export function getDetailSections(
  target: string,
  mode: 'edit' | 'create',
  placement: DetailSectionPlacement = 'after',
): DetailSectionEntry[] {
  return sections
    .filter((s) => s.target === target && (s.placement ?? 'after') === placement)
    .filter((s) => {
      const m = s.mode ?? 'edit'
      return m === 'both' || m === mode
    })
    .slice()
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
}

export function hasDetailSections(target: string, mode: 'edit' | 'create', placement: DetailSectionPlacement): boolean {
  return getDetailSections(target, mode, placement).length > 0
}

export function __resetDetailSectionsForTests() {
  sections.length = 0
}
