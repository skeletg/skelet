import { render } from '@testing-library/react'
import { act } from 'react'
import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { PrefsContext } from '@/lib/admin-prefs'
import { TabPanelContext } from '@/lib/tab-panel-context'
import { useRecordForm, type RecordForm } from './useRecordForm'

// What this pins: a schema built from data that loads after the page (a create page's default locale)
// re-runs when it changes. The error the half-loaded schema raised used to stick until the next edit,
// so the create refused with "Pick a locale" while the field on screen showed one

type Values = { name: string; locale: string }
const baseline: Values = { name: '', locale: '' }

const schemaFor = (fallback: string) =>
  z.object({ name: z.string(), locale: z.string().refine((v) => (v || fallback) !== '', 'Pick a locale') })

function Harness({ fallback, onApi }: { fallback: string; onApi: (r: RecordForm) => void }) {
  const record = useRecordForm<Values>({
    id: 'thing',
    title: 'Thing',
    label: 'New thing',
    isEdit: false,
    baseline,
    schema: schemaFor(fallback),
    diff: () => [],
    persist: false,
    submit: async () => undefined,
  })
  onApi(record as unknown as RecordForm)
  return null
}

function mount(fallback: string) {
  let api!: RecordForm
  const tree = (fb: string) => (
    <PrefsContext.Provider value="admin-1">
      <TabPanelContext.Provider value={{ active: true, visible: true, id: 'tab-a', paneId: 'p' }}>
        <Harness fallback={fb} onApi={(r) => (api = r)} />
      </TabPanelContext.Provider>
    </PrefsContext.Provider>
  )
  const view = render(tree(fallback))
  return { get: () => api, rerender: (fb: string) => view.rerender(tree(fb)) }
}

describe('useRecordForm', () => {
  it('re-runs a schema that changed after the admin typed', async () => {
    const { get, rerender } = mount('')
    await act(async () => {
      get().form.setFieldValue('name', 'Typed before the default locale loaded')
      await get().form.validate('change')
    })
    expect(get().form.state.isValid).toBe(false)

    // The default locale arrives
    await act(async () => rerender('en'))
    expect(get().form.state.isValid).toBe(true)
    // and the create goes through
    await act(async () => get().save())
    expect(get().submitError).toBeNull()
    expect(get().form.state.isSubmitSuccessful).toBe(true)
  })
})
