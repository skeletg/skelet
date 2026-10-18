import { render } from '@testing-library/react'
import { act } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { z } from 'zod'
import type { AnyFormApi } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import { SaveRegistryContext, useSaveRegistry } from './saveRegistry'
import { changed, fieldDiff } from './diff'
import { useFormSection } from './useFormSection'

// What these pin: a TanStack form lights the header Save for ANY field it owns (nested ones
// included - the hand-listed diffs missed those), and a rejected save rejects saveAll, so the
// confirm dialog stays open instead of closing as if it had worked

const schema = z.object({
  name: z.string().min(1, 'Name is required'),
  hours: z.object({ mon: z.string() }),
})
type Values = z.infer<typeof schema>
const baseline: Values = { name: 'Main', hours: { mon: '09:00' } }

type Api = {
  registry: ReturnType<typeof useSaveRegistry>
  form: AnyFormApi
}

function Section({ onSubmit, onForm }: { onSubmit: (v: Values) => Promise<void>; onForm: (f: AnyFormApi) => void }) {
  const form = useAppForm({
    defaultValues: baseline,
    validators: { onChange: schema },
    onSubmit: async ({ value }) => onSubmit(value),
  })
  useFormSection({
    id: 'loc',
    title: 'Location',
    form,
    baseline,
    diff: (a, b) => [
      ...fieldDiff(a, b, [{ label: 'Name', value: (o) => o.name }]),
      ...[changed('Hours · Mon', a.hours.mon, b.hours.mon)].filter((e) => e !== null),
    ],
  })
  onForm(form)
  return null
}

function Page(props: {
  onSubmit: (v: Values) => Promise<void>
  onRegistry: (r: Api['registry']) => void
  onForm: (f: AnyFormApi) => void
}) {
  const registry = useSaveRegistry()
  props.onRegistry(registry)
  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <Section onSubmit={props.onSubmit} onForm={props.onForm} />
    </SaveRegistryContext.Provider>
  )
}

function mount(onSubmit: (v: Values) => Promise<void> = async () => {}) {
  let registry!: Api['registry']
  let form!: AnyFormApi
  const view = render(
    <Page onSubmit={onSubmit} onRegistry={(r) => (registry = r)} onForm={(f) => (form = f)} />,
  )
  return { view, get: (): Api => ({ registry, form }) }
}

describe('useFormSection', () => {
  it('stays clean until a value moves', () => {
    const { view, get } = mount()
    expect(get().registry.isDirty).toBe(false)
    view.unmount()
  })

  it('lists a nested-only edit', () => {
    const { view, get } = mount()
    act(() => get().form.setFieldValue('hours.mon', 'closed'))
    expect(get().registry.isDirty).toBe(true)
    expect(get().registry.dirtySections[0].diff).toEqual([
      { kind: 'change', label: 'Hours · Mon', before: '09:00', after: 'closed' },
    ])
    view.unmount()
  })

  it('rejects saveAll when validation fails, without calling the mutation', async () => {
    const onSubmit = vi.fn(async () => {})
    const { view, get } = mount(onSubmit)
    act(() => get().form.setFieldValue('name', ''))
    await act(async () => {
      await expect(get().registry.saveAll()).rejects.toThrow(/Name is required/)
    })
    expect(onSubmit).not.toHaveBeenCalled()
    view.unmount()
  })

  it('rejects saveAll when the mutation throws', async () => {
    const { view, get } = mount(async () => {
      throw new Error('server said no')
    })
    act(() => get().form.setFieldValue('name', 'Renamed'))
    await act(async () => {
      await expect(get().registry.saveAll()).rejects.toThrow('server said no')
    })
    view.unmount()
  })

  it('resolves and submits the staged values when valid', async () => {
    const onSubmit = vi.fn(async () => {})
    const { view, get } = mount(onSubmit)
    act(() => get().form.setFieldValue('name', 'Renamed'))
    await act(async () => {
      await get().registry.saveAll()
    })
    expect(onSubmit).toHaveBeenCalledWith({ name: 'Renamed', hours: { mon: '09:00' } })
    view.unmount()
  })
})
