import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConfigFieldData } from '@/features/settings/ConfigField'
import type { SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { useConfigDrafts, type ConfigSectionData } from './useConfigDrafts'

// What is under test is the staging in between: which edits count as pending, how they read in the
// confirm dialog, and what actually reaches the wire. The two mutations, the save registry and the
// unsaved-changes guard are all captured rather than run
const { state } = vi.hoisted(() => ({
  state: {
    calls: [] as { name: string; variables: unknown }[],
    fail: {} as Record<string, string>,
    section: null as { diff: SectionDiffEntry[]; save: () => Promise<void> } | null,
    guard: { dirty: false, label: '' },
  },
}))

vi.mock('@apollo/client/react', () => ({
  useMutation: (doc: unknown) => {
    const name =
      (doc as { definitions?: { name?: { value?: string } }[] })?.definitions?.[0]?.name?.value ?? 'unknown'
    return [
      (opts: { variables?: unknown } = {}) => {
        state.calls.push({ name, variables: opts.variables })
        const failure = state.fail[name]
        return failure ? Promise.reject(new Error(failure)) : Promise.resolve({})
      },
      {},
    ]
  },
}))
vi.mock('@/shared/saving/saveRegistry', () => ({
  useSaveSection: (reg: { diff: SectionDiffEntry[]; save: () => Promise<void> }) => {
    state.section = reg
  },
}))
vi.mock('@/lib/config-scope', () => ({
  useUnsavedChangesGuard: (dirty: boolean, label: string) => {
    state.guard = { dirty, label }
  },
}))

const field = (over: Partial<ConfigFieldData> = {}): ConfigFieldData => ({
  path: 'general/store/name',
  section: 'general',
  group: 'store',
  field: 'name',
  label: 'Store name',
  type: 'STRING',
  value: 'Acme',
  isSet: false,
  isDefault: false,
  public: true,
  scoping: 'GLOBAL',
  scope: { kind: 'DEFAULT' },
  ...over,
})

const sections = (...fields: ConfigFieldData[]): ConfigSectionData[] => [
  { slug: 'general', label: 'General', groups: [{ slug: 'store', label: 'Store', fields }] },
]

const SCOPE = { kind: 'CHANNEL' as const, channelId: 'c1' }

function mount(fields: ConfigFieldData[]) {
  return renderHook(() => useConfigDrafts({ sections: sections(...fields), scopeInput: SCOPE }))
}

const setCalls = () => state.calls.filter((c) => /^SetConfigValue$/i.test(c.name))
const diff = () => state.section?.diff ?? []
const save = () => state.section!.save()

beforeEach(() => {
  state.calls = []
  state.fail = {}
  state.section = null
  state.guard = { dirty: false, label: '' }
})

describe('what counts as a pending edit', () => {
  it('stages nothing before anything is typed', () => {
    mount([field()])
    expect(diff()).toEqual([])
    expect(state.guard.dirty).toBe(false)
  })

  it('stages a real change, and arms the discard guard with it', () => {
    const { result } = mount([field()])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    expect(diff()).toHaveLength(1)
    expect(state.guard).toEqual({ dirty: true, label: 'Settings' })
  })

  it('drops an edit typed back to the stored value', () => {
    const { result } = mount([field({ value: 'Acme' })])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    act(() => result.current.onFieldChange('general/store/name', 'Acme'))
    expect(diff()).toEqual([])
    expect(state.guard.dirty).toBe(false)
  })

  it('clears a draft outright when the field hands back undefined', () => {
    const { result } = mount([field()])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    act(() => result.current.onFieldChange('general/store/name', undefined))
    expect(result.current.drafts).toEqual({})
    expect(diff()).toEqual([])
  })

  it('ignores a draft for a path no longer on screen', () => {
    const { result } = mount([field()])
    act(() => result.current.onFieldChange('gone/away/entirely', 'x'))
    expect(diff()).toEqual([])
  })
})

describe('how a change reads in the confirm dialog', () => {
  it('spells a boolean out rather than showing true and false', () => {
    const { result } = mount([field({ type: 'BOOL', value: 'false', label: 'Guest checkout' })])
    act(() => result.current.onFieldChange('general/store/name', 'true'))
    expect(diff()[0]).toMatchObject({ label: 'General · Guest checkout', before: 'Disabled', after: 'Enabled' })
  })

  it('shows a select’s option label, not the key it stores', () => {
    const f = field({
      type: 'SELECT',
      value: 'a',
      options: [{ label: 'Warehouse pickup', value: 'a' }, { label: 'Courier', value: 'b' }],
    })
    const { result } = mount([f])
    act(() => result.current.onFieldChange('general/store/name', 'b'))
    expect(diff()[0]).toMatchObject({ before: 'Warehouse pickup', after: 'Courier' })
  })

  it('falls back to the raw key when a select has no option for it', () => {
    const { result } = mount([field({ type: 'SELECT', value: 'a', options: [] })])
    act(() => result.current.onFieldChange('general/store/name', 'mystery'))
    expect(diff()[0]).toMatchObject({ after: 'mystery' })
  })

  it('never puts a secret on either side of the diff', () => {
    const { result } = mount([field({ type: 'SECRET', value: '', isSet: true, label: 'API key' })])
    act(() => result.current.onFieldChange('general/store/name', 'hunter2'))
    expect(diff()[0]).toMatchObject({ before: 'configured (hidden)', after: 'new secret (hidden)' })
    expect(JSON.stringify(diff())).not.toContain('hunter2')
  })

  it('says so when no secret is configured yet', () => {
    const { result } = mount([field({ type: 'SECRET', value: '', isSet: false })])
    act(() => result.current.onFieldChange('general/store/name', 'hunter2'))
    expect(diff()[0]).toMatchObject({ before: 'not set' })
  })
})

describe('saving', () => {
  it('sends one write per pending field, carrying the scope', async () => {
    const { result } = mount([field(), field({ path: 'general/store/city', field: 'city', value: 'NY' })])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    act(() => result.current.onFieldChange('general/store/city', 'LA'))
    await act(async () => { await save() })

    expect(setCalls()).toHaveLength(2)
    expect(setCalls().map((c) => c.variables)).toEqual(
      expect.arrayContaining([
        { path: 'general/store/name', value: 'Globex', scope: SCOPE },
        { path: 'general/store/city', value: 'LA', scope: SCOPE },
      ]),
    )
  })

  it('forgets the drafts once they are through', async () => {
    const { result } = mount([field()])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    await act(async () => { await save() })
    expect(result.current.drafts).toEqual({})
    expect(diff()).toEqual([])
  })

  // a blank secret means "leave the stored one alone" - sending '' would wipe it, and the only way
  // to clear a secret is the reset button
  it('refuses to send a blank secret over a configured one', async () => {
    const { result } = mount([field({ type: 'SECRET', value: '', isSet: true })])
    act(() => result.current.onFieldChange('general/store/name', '   '))
    expect(diff()).toEqual([])
    await act(async () => { await save() })
    expect(setCalls()).toEqual([])
  })

  it('rejects malformed JSON before anything reaches the wire', async () => {
    const { result } = mount([
      field({ type: 'JSON', value: '{}', label: 'Metadata' }),
      field({ path: 'general/store/city', field: 'city', value: 'NY' }),
    ])
    act(() => result.current.onFieldChange('general/store/name', '{ nope'))
    act(() => result.current.onFieldChange('general/store/city', 'LA'))

    await expect(act(async () => { await save() })).rejects.toThrow(/Metadata isn't valid JSON/)
    // the valid sibling must not have been written either - the whole save is refused
    expect(setCalls()).toEqual([])
  })

  it('accepts JSON that parses', async () => {
    const { result } = mount([field({ type: 'JSON', value: '{}' })])
    act(() => result.current.onFieldChange('general/store/name', '{"a":1}'))
    await act(async () => { await save() })
    expect(setCalls()).toHaveLength(1)
  })
})

describe('resetting a field to what it inherits', () => {
  it('asks the server, then drops any draft that was sitting on it', async () => {
    const { result } = mount([field()])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    await act(async () => { await result.current.onReset('general/store/name') })

    expect(state.calls.some((c) => /Reset/i.test(c.name))).toBe(true)
    expect(result.current.drafts).toEqual({})
    expect(result.current.resetError).toBeNull()
    expect(result.current.resettingPath).toBeNull()
  })

  it('surfaces a failure and leaves the draft alone', async () => {
    state.fail.ResetConfigValue = 'nope'
    const { result } = mount([field()])
    act(() => result.current.onFieldChange('general/store/name', 'Globex'))
    await act(async () => { await result.current.onReset('general/store/name') })

    expect(result.current.resetError).toBe('nope')
    expect(result.current.resettingPath).toBeNull() // the spinner must not be left running
    expect(result.current.drafts).toEqual({ 'general/store/name': 'Globex' })
  })
})
