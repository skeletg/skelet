import { useEffect } from 'react'
import { act, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionRegistration,
} from './saveRegistry'
import { hasUnsavedChanges, unsavedLabels } from '@/lib/unsaved-changes'

// The staged-save registry is THE dashboard convention - these pin its load-bearing semantics:
// dirty filtering, sequential saves, unregister-on-unmount, and the stale-draft protection where
// save thunks re-read the latest registration instead of the closure they were registered with

type Api = ReturnType<typeof useSaveRegistry>

function Section({ reg }: { reg: SectionRegistration }) {
  useSaveSection(reg)
  return null
}

function Harness({
  sections,
  onApi,
}: {
  sections: SectionRegistration[]
  onApi: (api: Api) => void
}) {
  const api = useSaveRegistry()
  useEffect(() => {
    onApi(api)
  })
  return (
    <SaveRegistryContext.Provider value={api.contextValue}>
      {sections.map((s) => (
        <Section key={s.id} reg={s} />
      ))}
    </SaveRegistryContext.Provider>
  )
}

const section = (
  id: string,
  diff: SectionRegistration['diff'],
  save: SectionRegistration['save'] = () => Promise.resolve(),
): SectionRegistration => ({ id, title: id, diff, save })

describe('save registry', () => {
  it('only sections with a non-empty diff count as dirty', () => {
    let api!: Api
    render(
      <Harness
        onApi={(a) => (api = a)}
        sections={[
          section('clean', []),
          section('dirty', [{ label: 'Name', before: 'a', after: 'b' }]),
        ]}
      />,
    )
    expect(api.isDirty).toBe(true)
    expect(api.dirtySections.map((s) => s.id)).toEqual(['dirty'])
  })

  it('saveAll runs dirty sections sequentially and skips clean ones', async () => {
    const order: string[] = []
    const slowSave = (id: string) => async () => {
      await new Promise((r) => setTimeout(r, 5))
      order.push(id)
    }
    let api!: Api
    render(
      <Harness
        onApi={(a) => (api = a)}
        sections={[
          section('one', [{ label: 'x', before: '', after: '1' }], slowSave('one')),
          section('clean', [], slowSave('clean')),
          section('two', [{ label: 'y', before: '', after: '2' }], slowSave('two')),
        ]}
      />,
    )
    await act(() => api.saveAll())
    expect(order).toEqual(['one', 'two'])
  })

  it('unmounting a section unregisters it', () => {
    let api!: Api
    const dirty = section('gone', [{ label: 'x', before: 'a', after: 'b' }])
    const { rerender } = render(<Harness onApi={(a) => (api = a)} sections={[dirty]} />)
    expect(api.isDirty).toBe(true)
    rerender(<Harness onApi={(a) => (api = a)} sections={[]} />)
    expect(api.isDirty).toBe(false)
  })

  it('saves the LATEST draft even when the diff string did not change', async () => {
    // The registry only re-registers when the diff STRING changes, and a draft can drift without
    // changing it, so the save thunk must re-read the latest registration at call time
    const staleSave = vi.fn(() => Promise.resolve())
    const freshSave = vi.fn(() => Promise.resolve())
    const diff = [{ label: 'Body', before: 'previous', after: 'updated' }]

    let api!: Api
    const { rerender } = render(
      <Harness onApi={(a) => (api = a)} sections={[section('rich', diff, staleSave)]} />,
    )
    // Same diff content, new save closure - simulates a further edit that
    // the summarized diff string can't distinguish
    rerender(
      <Harness onApi={(a) => (api = a)} sections={[section('rich', [...diff], freshSave)]} />,
    )
    await act(() => api.saveAll())
    expect(freshSave).toHaveBeenCalledTimes(1)
    expect(staleSave).not.toHaveBeenCalled()
  })
})

// The bridge that gives every staged-save page a discard prompt without the page opting in - if
// this breaks, 24 detail pages go back to losing edits silently on a scope switch or a refresh
describe('unsaved-changes bridge', () => {
  it('publishes a dirty page to the discard guards, and withdraws it when clean', () => {
    const dirty = section('one', [{ label: 'Name', before: 'a', after: 'b' }])
    const { rerender, unmount } = render(<Harness onApi={() => {}} sections={[dirty]} />)
    expect(hasUnsavedChanges()).toBe(true)

    rerender(<Harness onApi={() => {}} sections={[section('one', [])]} />)
    expect(hasUnsavedChanges()).toBe(false)

    rerender(<Harness onApi={() => {}} sections={[dirty]} />)
    expect(hasUnsavedChanges()).toBe(true)
    unmount()
    expect(hasUnsavedChanges()).toBe(false)
  })

  it('passes the page label through to the prompt', () => {
    function Labelled() {
      const api = useSaveRegistry('Product \'Acme Widget\'')
      return (
        <SaveRegistryContext.Provider value={api.contextValue}>
          <Section reg={section('one', [{ label: 'Name', before: 'a', after: 'b' }])} />
        </SaveRegistryContext.Provider>
      )
    }
    const view = render(<Labelled />)
    expect(unsavedLabels()).toEqual(["Product 'Acme Widget'"])
    view.unmount()
  })
})
