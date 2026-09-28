import { render } from '@testing-library/react'
import { act } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AnyFormApi } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import { PrefsContext } from '@/lib/admin-prefs'
import { TabPanelContext } from '@/lib/tab-panel-context'
import { draftKey, readDraft, writeDraft } from '@/lib/form-drafts'
import { hasUnsavedChanges } from '@/lib/unsaved-changes'
import { usePersistedForm, type PersistedForm } from './usePersistedForm'

// What these pin: typed work survives a remount (the stand-in for a refresh) in the same tab as an
// OFFER - the form keeps the stored values until the admin restores - a record the server changed
// meanwhile is reported rather than silently overwritten, an edit made instead of deciding replaces
// the draft, saving and discarding both drop it, and excluded fields never reach storage

type Values = { name: string; city: string; apiKey: string }
const KEY = draftKey('admin-1', 'tab-a', 'loc:1')

type Api = { form: AnyFormApi; draft: PersistedForm }

function Harness({ baseline, onApi }: { baseline: Values; onApi: (a: Api) => void }) {
  const form = useAppForm({ defaultValues: baseline })
  const draft = usePersistedForm(form, { routeKey: 'loc:1', baseline, exclude: ['apiKey'] })
  onApi({ form, draft })
  return null
}

function mount(baseline: Values) {
  let api!: Api
  const view = render(
    <PrefsContext.Provider value="admin-1">
      <TabPanelContext.Provider value={{ active: true, visible: true, id: 'tab-a', paneId: 'p' }}>
        <Harness baseline={baseline} onApi={(a) => (api = a)} />
      </TabPanelContext.Provider>
    </PrefsContext.Provider>,
  )
  return { view, get: () => api }
}

const server: Values = { name: 'Main', city: 'Berlin', apiKey: '' }

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  localStorage.clear()
})

function settle() {
  act(() => {
    vi.advanceTimersByTime(500)
  })
}

describe('usePersistedForm', () => {
  it('writes only the moved, non-excluded fields after the debounce', () => {
    const { view, get } = mount(server)
    act(() => {
      get().form.setFieldValue('city', 'Hamburg')
      get().form.setFieldValue('apiKey', 'sk-secret')
    })
    expect(readDraft(KEY)).toBeNull() // still inside the debounce
    settle()
    expect(readDraft(KEY)?.touched).toEqual({ city: { base: 'Berlin', value: 'Hamburg' } })
    view.unmount()
  })

  it('offers a stored draft on a fresh mount without applying it', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount(server)
    expect(get().form.state.values.city).toBe('Berlin')
    expect(get().draft.offer?.rows).toEqual([expect.objectContaining({ label: 'city', before: 'Berlin', after: 'Hamburg' })])
    // an offer is not unsaved work yet - nothing prompts for it
    expect(hasUnsavedChanges('tab-a', { includePersisted: true })).toBe(false)
    // and a reload before deciding offers it again
    settle()
    expect(readDraft(KEY)?.touched).toEqual({ city: { base: 'Berlin', value: 'Hamburg' } })
    view.unmount()
  })

  it('restore puts the draft into the form and marks the work persisted', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount(server)
    act(() => get().draft.restore())
    expect(get().form.state.values.city).toBe('Hamburg')
    expect(get().draft.offer).toBeNull()
    // refresh/close don't prompt for it - closing the tab still does
    expect(hasUnsavedChanges()).toBe(false)
    expect(hasUnsavedChanges('tab-a', { includePersisted: true })).toBe(true)
    view.unmount()
  })

  it('reports a clash before restoring and keeps the admin value after', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount({ ...server, city: 'Munich' })
    expect(get().draft.offer?.clashes).toEqual([{ field: 'city', server: 'Munich' }])
    act(() => get().draft.restore())
    expect(get().form.state.values.city).toBe('Hamburg')
    expect(get().draft.clashes).toEqual([{ field: 'city', server: 'Munich' }])
    view.unmount()
  })

  it('an edit made instead of deciding drops the offer and becomes the draft', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount(server)
    act(() => get().form.setFieldValue('name', 'Annex'))
    expect(get().draft.offer).toBeNull()
    settle()
    expect(readDraft(KEY)?.touched).toEqual({ name: { base: 'Main', value: 'Annex' } })
    view.unmount()
  })

  it('drops a draft the server already caught up with', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount({ ...server, city: 'Hamburg' })
    expect(get().draft.offer).toBeNull()
    expect(readDraft(KEY)).toBeNull()
    view.unmount()
  })

  it('discard goes back to the baseline and forgets the draft', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount(server)
    act(() => get().draft.restore())
    act(() => get().draft.discard())
    settle()
    expect(get().form.state.values.city).toBe('Berlin')
    expect(get().draft.offer).toBeNull()
    expect(readDraft(KEY)).toBeNull()
    view.unmount()
  })

  it('saved() drops the draft and does not write it back before the refetch lands', () => {
    const { view, get } = mount(server)
    act(() => get().form.setFieldValue('city', 'Hamburg'))
    settle()
    expect(readDraft(KEY)).not.toBeNull()
    act(() => get().draft.saved())
    settle()
    expect(readDraft(KEY)).toBeNull()
    expect(hasUnsavedChanges('tab-a', { includePersisted: true })).toBe(false)
    view.unmount()
  })
})
