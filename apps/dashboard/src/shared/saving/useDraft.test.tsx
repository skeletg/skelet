import { render } from '@testing-library/react'
import { act } from 'react'
import { describe, expect, it } from 'vitest'
import { useDraft, type Draft } from './useDraft'
import { hasUnsavedChanges, unsavedLabels } from '@/lib/unsaved-changes'

// The point of the container is that dirt is complete by construction - no field list to forget -
// and that a page which saves and STAYS can go clean again, which a hand-listed watch cannot

type Fields = { name: string; count: number; tags: string[] }

function Harness({ onDraft }: { onDraft: (d: Draft<Fields>) => void }) {
  const draft = useDraft<Fields>({ name: 'mounted', count: 1, tags: [] }, 'Thing')
  onDraft(draft)
  return null
}

function mount() {
  let draft!: Draft<Fields>
  const view = render(<Harness onDraft={(d) => (draft = d)} />)
  return { view, get: () => draft }
}

describe('useDraft', () => {
  it('starts clean at its mounted values', () => {
    const { view, get } = mount()
    expect(get().dirty).toBe(false)
    expect(hasUnsavedChanges()).toBe(false)
    view.unmount()
  })

  it('generates setters named like the useState pairs they replace', () => {
    const { view, get } = mount()
    act(() => get().setters.setName('typed'))
    expect(get().values.name).toBe('typed')
    expect(get().dirty).toBe(true)
    expect(unsavedLabels()).toEqual(['Thing'])
    view.unmount()
  })

  it('takes the updater form, the way setState does', () => {
    const { view, get } = mount()
    act(() => get().setters.setTags((prev) => [...prev, 'a']))
    act(() => get().setters.setCount((prev) => prev + 4))
    expect(get().values.tags).toEqual(['a'])
    expect(get().values.count).toBe(5)
    view.unmount()
  })

  it('goes clean again when a field is put back', () => {
    const { view, get } = mount()
    act(() => get().setters.setName('typed'))
    expect(hasUnsavedChanges()).toBe(true)
    act(() => get().setters.setName('mounted'))
    expect(hasUnsavedChanges()).toBe(false)
    view.unmount()
  })

  it('re-baselines on saved(), so an editor that stays put stops nagging', () => {
    const { view, get } = mount()
    act(() => get().setters.setName('persisted'))
    expect(hasUnsavedChanges()).toBe(true)
    act(() => get().saved())
    expect(get().values.name).toBe('persisted') // keeps what is on screen
    expect(hasUnsavedChanges()).toBe(false)
    // and edits AFTER the save count again, against the new baseline
    act(() => get().setters.setName('edited again'))
    expect(hasUnsavedChanges()).toBe(true)
    view.unmount()
  })

  it('reset() throws the edits away', () => {
    const { view, get } = mount()
    act(() => get().patch({ name: 'x', count: 9 }))
    act(() => get().reset())
    expect(get().values).toEqual({ name: 'mounted', count: 1, tags: [] })
    expect(hasUnsavedChanges()).toBe(false)
    view.unmount()
  })

  it('reset(next) adopts a fresh baseline, e.g. a refetched record', () => {
    const { view, get } = mount()
    act(() => get().reset({ name: 'from server', count: 7, tags: ['t'] }))
    expect(get().dirty).toBe(false)
    act(() => get().setters.setName('mounted')) // the OLD mount value is now an edit
    expect(get().dirty).toBe(true)
    view.unmount()
  })

  it('unregisters on unmount', () => {
    const { view, get } = mount()
    act(() => get().setters.setName('typed'))
    expect(hasUnsavedChanges()).toBe(true)
    view.unmount()
    expect(hasUnsavedChanges()).toBe(false)
  })
})
