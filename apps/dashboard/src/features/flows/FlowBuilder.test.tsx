import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { LibraryEmailNamesDocument } from '@/features/emails/Emails.gql'
import { FlowBuilder } from './FlowBuilder'
import { CreateFlowDocument, FlowSegmentsDocument, FlowTriggersDocument } from './Flows.gql'

// What the suite says of each trigger: whether its flows may be transactional, and whether they carry a one-time link
const triggers = [
  { __typename: 'FlowTrigger', type: 'order_placed', transactional: true, oneTimeLink: false },
  { __typename: 'FlowTrigger', type: 'cart_abandoned', transactional: false, oneTimeLink: false },
  { __typename: 'FlowTrigger', type: 'password_reset', transactional: true, oneTimeLink: true },
]

const store = { id: 'ch1', name: 'Demo Store', defaultLocale: 'en', locales: [{ code: 'en' }], isActive: true }
const inStore: ConfigScopeContextValue = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'ch1', locale: '' },
  channels: [store],
  selectedChannel: store,
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'ch1',
}

const segments = { request: { query: FlowSegmentsDocument }, result: { data: { customerSegments: { __typename: 'CustomerSegmentList', items: [] } } }, maxUsageCount: 5 }

const reads = () => [
  meMock(),
  { request: { query: FlowTriggersDocument }, result: { data: { flowTriggers: triggers } }, maxUsageCount: 5 },
  segments,
  { request: { query: LibraryEmailNamesDocument, variables: { channelId: 'ch1' } }, result: { data: { libraryEmails: [] } }, maxUsageCount: 5 },
]

const newFlow = (mocks = reads(), guarded = false) => renderPage({ path: '/flows/new', scope: inStore, guarded, routes: { '/flows/new': <FlowBuilder />, '/flows/:id': <p>Saved flow</p> }, mocks })

const stepCount = () => screen.getByText('Steps', { selector: 'span' }).nextElementSibling?.textContent

const STEP_DRAG = 'application/x-skelet-flow-step'
const dropOnCanvas = (type: string) => fireEvent.drop(document.querySelector('[data-flow-canvas]')!, { dataTransfer: { getData: (k: string) => (k === STEP_DRAG ? type : ''), types: [STEP_DRAG] } })

// What this pins: the panel follows the canvas - adding a step from the rail opens its settings
describe('FlowBuilder layout', () => {
  it('adds a step from the rail and opens its settings', async () => {
    newFlow()

    const rail = await screen.findByRole('complementary', { name: 'Steps' }, SLOW)
    expect(rail).toBeDefined()
    expect(screen.getByRole('tab', { name: 'Flow' }).getAttribute('aria-selected')).toBe('true')
    expect(screen.getByLabelText('Flow name')).toBeDefined()
    // On the trigger step in the canvas, and as what the flow starts on in the panel
    expect(screen.getAllByText('Order placed').length).toBe(2)

    fireEvent.click(screen.getByRole('button', { name: 'Add Wait' }))
    expect(screen.getByRole('tab', { name: 'Step' }).getAttribute('aria-selected')).toBe('true')
    expect(await screen.findByLabelText('Wait for')).toBeDefined()
    expect(screen.getByText('Hold the customer here for a set time.')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Delete step' })).toBeDefined()
  })

  // What this pins: a recipe builds and names the flow, the checks hold Activate back, and one undo is the whole recipe
  it('starts from a recipe, says what is left to do, and undoes it', async () => {
    newFlow()

    const recipe = await screen.findByRole('button', { name: /^Abandoned cart/ }, SLOW)
    expect(screen.queryByRole('button', { name: /^Welcome series/ })).toBeNull()
    const activate = screen.getByRole('button', { name: 'Activate' })
    expect(activate).toHaveProperty('disabled', true)
    expect(screen.getByText('Nothing follows the trigger, so nobody would be enrolled.')).toBeDefined()
    expect(screen.getByText(/^A transactional flow/)).toBeDefined()

    fireEvent.click(recipe)
    expect(screen.getByText(/^A marketing flow/)).toBeDefined()
    expect(screen.getByLabelText('Flow name')).toHaveProperty('value', 'Abandoned cart')
    expect(screen.getAllByText('Pick the email to send.').length).toBe(2)
    expect(screen.getByRole('button', { name: /^Checks/ }).textContent).toContain('2 to fix')
    expect(activate).toHaveProperty('disabled', true)
    expect(screen.queryByRole('region', { name: 'Start from a recipe' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Undo' }))
    expect(await screen.findByRole('region', { name: 'Start from a recipe' })).toBeDefined()
    expect(screen.getByLabelText('Flow name')).toHaveProperty('value', '')
  })

  // Which triggers a recipe may start on is the server's call, though the builder has the recipe
  it('offers no recipe on a trigger the server keeps transactional', async () => {
    const all = triggers.map((t) => ({ ...t, transactional: true }))
    newFlow([meMock(), { request: { query: FlowTriggersDocument }, result: { data: { flowTriggers: all } }, maxUsageCount: 5 }, ...reads().slice(2)])
    await screen.findByRole('complementary', { name: 'Steps' }, SLOW)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Change' })).toBeDefined())
    expect(screen.queryByRole('region', { name: 'Start from a recipe' })).toBeNull()
  })

  it('says so when a list it relies on did not load', async () => {
    newFlow([...reads().filter((m) => m !== segments), { request: { query: FlowSegmentsDocument }, error: new Error('down') }])
    expect(await screen.findByText("Couldn't load the segments. Reload the page to try again.", {}, SLOW)).toBeDefined()
  })
})

// What this pins: the canvas takes a step and nothing else, and a field keeps its own undo
describe('FlowBuilder canvas', () => {
  it('takes a step dropped on it and refuses anything that is not one', async () => {
    newFlow()
    await screen.findByRole('complementary', { name: 'Steps' }, SLOW)
    expect(stepCount()).toBe('0')

    dropOnCanvas('trigger')
    dropOnCanvas('<img src=x>')
    expect(stepCount()).toBe('0')

    dropOnCanvas('wait')
    expect(stepCount()).toBe('1')
    expect(await screen.findByLabelText('Wait for')).toBeDefined()
  })

  it('undoes and redoes from the keyboard, but not while typing in a field', async () => {
    newFlow()
    fireEvent.click(await screen.findByRole('button', { name: 'Add Wait' }, SLOW))
    expect(stepCount()).toBe('1')

    fireEvent.keyDown(screen.getByLabelText('Wait for'), { key: 'z', ctrlKey: true })
    expect(stepCount()).toBe('1')

    fireEvent.keyDown(document.body, { key: 'z', ctrlKey: true })
    expect(stepCount()).toBe('0')
    fireEvent.keyDown(document.body, { key: 'z', ctrlKey: true, shiftKey: true })
    expect(stepCount()).toBe('1')
  })

  it('saves a new flow and leaves for it without asking about unsaved changes', async () => {
    const asked = vi.spyOn(window, 'confirm').mockReturnValue(false)
    let saved: Record<string, unknown> | undefined
    const flow = {
      __typename: 'Flow', id: 'f1', channelId: 'ch1', name: 'After an order', status: 'draft', triggerType: 'order_placed', triggerConfig: { transactional: true },
      graph: { nodes: [], edges: [] }, createdAt: '2026-11-01T00:00:00Z', updatedAt: '2026-11-01T00:00:00Z', deletedAt: null,
    }
    newFlow([...reads(), { request: { query: CreateFlowDocument, variables: captureInput((input) => (saved = input)) }, result: { data: { createFlow: flow } } }], true)
    fireEvent.change(await screen.findByLabelText('Flow name', {}, SLOW), { target: { value: 'After an order' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add Wait' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('Saved flow', {}, SLOW)).toBeDefined()
    expect(asked).not.toHaveBeenCalled()
    expect(saved).toMatchObject({ channelId: 'ch1', name: 'After an order', triggerType: 'order_placed', triggerConfig: { transactional: true } })
    const graph = saved?.graph as { nodes: Array<{ type: string }>; edges: unknown[] }
    expect(graph.nodes.map((n) => n.type)).toEqual(['trigger', 'wait'])
    expect(graph.edges.length).toBe(1)
    asked.mockRestore()
  })
})
