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

// What these pin: typed work survives a remount (the stand-in for a refresh) in the same tab, a
// record the server changed meanwhile is reported rather than silently overwritten, saving and
// discarding both drop the draft, and excluded fields never reach storage

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

  it('restores into a fresh mount and marks the work persisted', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount(server)
    expect(get().form.state.values.city).toBe('Hamburg')
    expect(get().draft.restoredAt).not.toBeNull()
    expect(get().draft.clashes).toEqual([])
    // refresh/close don't prompt for it - closing the tab still does
    expect(hasUnsavedChanges()).toBe(false)
    expect(hasUnsavedChanges('tab-a', { includePersisted: true })).toBe(true)
    view.unmount()
  })

  it('keeps the admin value on a clash and reports what the server holds now', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount({ ...server, city: 'Munich' })
    expect(get().form.state.values.city).toBe('Hamburg')
    expect(get().draft.clashes).toEqual([{ field: 'city', server: 'Munich' }])
    view.unmount()
  })

  it('drops a draft the server already caught up with', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount({ ...server, city: 'Hamburg' })
    expect(get().draft.restoredAt).toBeNull()
    expect(readDraft(KEY)).toBeNull()
    view.unmount()
  })

  it('discard goes back to the baseline and forgets the draft', () => {
    writeDraft(KEY, { city: { base: 'Berlin', value: 'Hamburg' } })
    const { view, get } = mount(server)
    act(() => get().draft.discard())
    settle()
    expect(get().form.state.values.city).toBe('Berlin')
    expect(get().draft.restoredAt).toBeNull()
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
