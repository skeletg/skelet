import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { AssistantVoice } from './AssistantVoice'

const store = {
  ...globalScope,
  scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' },
  scopeInput: { kind: 'CHANNEL' as const, channelId: 'ch1' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'ch1',
  selectedChannel: { id: 'ch1', name: 'Main' },
} as typeof globalScope

const field = (path: string, label: string, type: string, value: string, extra: object = {}) => ({
  __typename: 'ConfigField' as const,
  path,
  section: 'sales',
  group: path.split('/')[1],
  field: path.split('/').pop(),
  label,
  description: '',
  type,
  value,
  isSet: false,
  maxLength: null,
  options: [],
  isDefault: true,
  public: false,
  scoping: 'CHANNEL' as const,
  scope: { __typename: 'ConfigScope' as const, kind: 'DEFAULT' as const, channelId: null, locale: null },
  updatedAt: null,
  ...extra,
})

const ownTone = { isDefault: false, scope: { __typename: 'ConfigScope' as const, kind: 'CHANNEL' as const, channelId: 'ch1', locale: null } }

const sections = (assistantOn: boolean, toneSetHere = false) => ({
  request: { query: ConfigSectionsDocument, variables: { scope: { kind: 'CHANNEL', channelId: 'ch1' } } },
  maxUsageCount: 3,
  result: {
    data: {
      configSections: [
        {
          __typename: 'ConfigSection' as const,
          slug: 'sales',
          label: 'Sales',
          groups: [
            { __typename: 'ConfigGroup' as const, slug: 'agents', label: 'AI shopping agents', fields: [field('sales/agents/assistant_enabled', 'Show an AI shopping assistant', 'BOOL', String(assistantOn))] },
            {
              __typename: 'ConfigGroup' as const,
              slug: 'assistant',
              label: "Shopping assistant's voice",
              fields: [
                field('sales/assistant/name', "The assistant's name", 'STRING', '', { maxLength: 40 }),
                field('sales/assistant/tone', 'How it sounds', 'SELECT', 'friendly', {
                  ...(toneSetHere ? ownTone : {}),
                  options: [
                    { __typename: 'ConfigOption', label: 'Friendly and warm', value: 'friendly' },
                    { __typename: 'ConfigOption', label: 'Brief and to the point', value: 'brief' },
                  ],
                }),
                field('sales/assistant/instructions', 'What it should know and follow', 'TEXTAREA', '', { maxLength: 4000 }),
              ],
            },
          ],
        },
      ],
    },
  },
})

const type = (el: HTMLElement, value: string) => {
  el.focus()
  fireEvent.change(el, { target: { value } })
}

afterEach(() => localStorage.clear())

describe('AssistantVoice', () => {
  it("stages the name and instructions behind the header Save, counting what's left", async () => {
    const writes = vi.fn()
    renderPage({
      path: '/agentic/voice',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/voice': <AssistantVoice /> },
      mocks: [
        meMock(),
        sections(true),
        {
          request: {
            query: SetConfigValueDocument,
            variables: (vars: Record<string, unknown>) => {
              writes(vars)
              return true
            },
          },
          maxUsageCount: 2,
          result: (vars: Record<string, unknown>) => ({
            data: {
              setConfigValue: {
                __typename: 'ConfigValue',
                path: vars.path,
                value: vars.value,
                isDefault: false,
                scope: { __typename: 'ConfigScope', kind: 'CHANNEL', channelId: 'ch1', locale: null },
                updatedAt: null,
              },
            },
          }),
        },
      ],
    })

    const instructions = await screen.findByLabelText('What it should know and follow')
    expect(instructions).toHaveAttribute('maxLength', '4000')
    expect(screen.queryByText('The assistant is off on this store')).toBeNull()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()

    type(screen.getByLabelText("The assistant's name"), 'Ava')
    type(instructions, 'Our jeans run small.')
    expect(screen.getByText('20 / 4,000')).toBeInTheDocument()
    expect(writes).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Our jeans run small.')).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(writes).toHaveBeenCalledTimes(2), SLOW)
    expect(writes).toHaveBeenCalledWith(expect.objectContaining({ path: 'sales/assistant/name', value: 'Ava', scope: { kind: 'CHANNEL', channelId: 'ch1' } }))
    expect(writes).toHaveBeenCalledWith(expect.objectContaining({ path: 'sales/assistant/instructions', value: 'Our jeans run small.' }))
    await vi.waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
  })

  it('picks a tone by its example, and previews the panel with the staged name and tone', async () => {
    const writes = vi.fn()
    renderPage({
      path: '/agentic/voice',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/voice': <AssistantVoice /> },
      mocks: [
        meMock(),
        sections(true),
        {
          request: {
            query: SetConfigValueDocument,
            variables: (vars: Record<string, unknown>) => {
              writes(vars)
              return true
            },
          },
          result: {
            data: {
              setConfigValue: {
                __typename: 'ConfigValue',
                path: 'sales/assistant/tone',
                value: 'brief',
                isDefault: false,
                scope: { __typename: 'ConfigScope', kind: 'CHANNEL', channelId: 'ch1', locale: null },
                updatedAt: null,
              },
            },
          },
        },
      ],
    })
    const preview = within((await screen.findByText('Anything for rainy hikes?')).closest('figure')!)
    expect(preview.getByText('Shopping assistant')).toBeInTheDocument()
    expect(preview.getByText(/^Happy to help!/)).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /Friendly and warm/ })).toHaveAttribute('aria-checked', 'true')

    type(screen.getByLabelText("The assistant's name"), 'Ava')
    fireEvent.click(screen.getByRole('radio', { name: /Brief and to the point/ }))
    expect(screen.getByRole('radio', { name: /Brief and to the point/ })).toHaveAttribute('aria-checked', 'true')
    expect(preview.getByText('Ava')).toBeInTheDocument()
    expect(preview.getByText('Two waterproof shells. The lighter one packs into its pocket.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('radio', { name: /Friendly and warm/ }))
    type(screen.getByLabelText("The assistant's name"), '')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    fireEvent.click(screen.getByRole('radio', { name: /Brief and to the point/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(writes).toHaveBeenCalledWith(expect.objectContaining({ path: 'sales/assistant/tone', value: 'brief' })), SLOW)
    expect(writes).toHaveBeenCalledTimes(1)
    await vi.waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
  })

  it('says when the assistant is off on the store, and where to switch it on', async () => {
    renderPage({ path: '/agentic/voice', adminId: 'admin-1', scope: store, routes: { '/agentic/voice': <AssistantVoice /> }, mocks: [meMock(), sections(false)] })
    expect(await screen.findByText('The assistant is off on this store')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Settings, Sales, AI shopping agents' })).toHaveAttribute('href', '/settings/sales/agents')
  })

  it("lets someone who can't manage settings read them but not change them", async () => {
    renderPage({
      path: '/agentic/voice',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/voice': <AssistantVoice /> },
      mocks: [meMock({ superuser: false, permissions: ['config.view'] }), sections(true, true)],
    })
    expect(await screen.findByLabelText('What it should know and follow')).toBeDisabled()
    expect(screen.getByText(/changing them needs permission/)).toBeInTheDocument()
    expect(screen.getByText('Set for this store')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Reset/ })).toBeNull()
  })

  it('offers a way back for a value set on this store', async () => {
    renderPage({ path: '/agentic/voice', adminId: 'admin-1', scope: store, routes: { '/agentic/voice': <AssistantVoice /> }, mocks: [meMock(), sections(true, true)] })
    expect(await screen.findByRole('button', { name: 'Reset How it sounds' })).toBeInTheDocument()
  })

  it('moves the tone with the arrow keys, wrapping at the ends', async () => {
    renderPage({ path: '/agentic/voice', adminId: 'admin-1', scope: store, routes: { '/agentic/voice': <AssistantVoice /> }, mocks: [meMock(), sections(true)] })
    const friendly = await screen.findByRole('radio', { name: /Friendly and warm/ })
    const brief = screen.getByRole('radio', { name: /Brief and to the point/ })
    fireEvent.keyDown(friendly, { key: 'ArrowRight' })
    expect(brief).toHaveAttribute('aria-checked', 'true')
    expect(brief).toHaveFocus()
    fireEvent.keyDown(brief, { key: 'ArrowDown' })
    expect(friendly).toHaveAttribute('aria-checked', 'true')
    expect(friendly).toHaveFocus()
  })

  it('focuses the first tone when the stored one is no longer offered', async () => {
    const stale = sections(true)
    const tone = stale.result.data.configSections[0].groups[1].fields[1]
    tone.value = 'retired'
    renderPage({ path: '/agentic/voice', adminId: 'admin-1', scope: store, routes: { '/agentic/voice': <AssistantVoice /> }, mocks: [meMock(), stale] })
    const radios = await screen.findAllByRole('radio')
    expect(radios.map((r) => r.getAttribute('tabindex'))).toEqual(['0', '-1'])
    expect(radios.every((r) => r.getAttribute('aria-checked') === 'false')).toBe(true)
  })
})
