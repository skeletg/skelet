import type { ReactNode } from 'react'

// Detail-page section registry - plugins call registerDetailSection at import time and each
// entry renders at the end of the matching entity's <DetailPageBody> via <PluginSections>
// A section owns its own state, mutations and save UX - wiring into the parent form's Save
// button is out of scope, use useSaveSection (shared/saving/saveRegistry) for that

export type DetailSectionMode = 'edit' | 'create' | 'both'

export type DetailSectionEntry<TEntity = unknown> = {
  id: string  // slug-style, conventionally '<plugin-name>:section' - duplicates throw at registration
  target: string  // free-form, so a new detail page needs no core registry change
  order?: number  // lower runs earlier, default 100 - core sections sit at 10-50
  mode?: DetailSectionMode  // default 'edit', since most plugin sections need an existing entity id
  render: (props: { entity: TEntity; mode: 'edit' | 'create' }) => ReactNode  // must produce a <DetailPageSection> at its root so the surrounding divide-y works
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
): DetailSectionEntry[] {
  return sections
    .filter((s) => s.target === target)
    .filter((s) => {
      const m = s.mode ?? 'edit'
      return m === 'both' || m === mode
    })
    .slice()
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
}

export function __resetDetailSectionsForTests() {
  sections.length = 0
}
