import { useState } from 'react'
import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { ConfigScopeContext } from '@/lib/config-scope'
import { AssistantNameDocument } from '@/features/agentic/Agentic.gql'
import { globalScope, meMock, renderPage } from '@/test/page-harness'
import { AssistantConversationDocument, AssistantConversationsDocument, AssistantTurnRoundsDocument } from './Conversations.gql'
import { ConversationDetail } from './ConversationDetail'
import { ConversationsList } from './ConversationsList'

const store = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' }, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1', selectedChannel: { id: 'ch1', name: 'Main' } } as typeof globalScope

const named = (channelId = 'ch1') => ({
  request: { query: AssistantNameDocument, variables: { scope: { kind: 'CHANNEL', channelId } } },
  result: { data: { configValue: { __typename: 'ConfigValue', path: 'sales/assistant/name', value: 'Ava' } } },
})
const signedIn = [meMock(), named()]

type Gap = { __typename: 'AssistantGap'; turnId: string; question: string; reasons: string[] } | null

const conversation = (id: string, firstQuestion: string, unanswered = 0, latestUnanswered: Gap = null) => ({
  __typename: 'AssistantConversation',
  id,
  customerId: null,
  anonId: 'visitor-1',
  firstQuestion,
  turns: 2,
  cartChanged: true,
  checkout: false,
  incomplete: false,
  startedAt: '2026-11-04T10:00:00Z',
  lastAt: '2026-11-04T10:05:00Z',
  unanswered,
  latestUnanswered,
})

const counts = { __typename: 'AssistantGapCounts', any: 4, noAnswer: 1, cutShort: 0, foundNothing: 3, unsure: 2 }

const page = (after: string | null, items: ReturnType<typeof conversation>[], nextCursor: string | null, channelId = 'ch1', search: string | null = null, unanswered: string | null = null) => ({
  request: { query: AssistantConversationsDocument, variables: { channelId, search, unanswered, after, limit: 30 } },
  result: { data: { assistantConversations: { __typename: 'AssistantConversationPage', items, counts, nextCursor } } },
})

describe('ConversationsList', () => {
  it('pages older conversations by the cursor the last page gave', async () => {
    const first = Array.from({ length: 30 }, (_, i) => conversation(`c${i}`, `question ${i}`))
    renderPage({
      path: '/conversations',
      routes: { '/conversations': <ConversationsList /> },
      // the first page twice - going back to it shows the cached rows and asks again
      mocks: [...signedIn, page(null, first, 'cursor-1'), page('cursor-1', [conversation('c30', 'an older question')], null), page(null, first, 'cursor-1')],
      scope: store,
      adminId: 'admin-1',
    })
    expect(await screen.findByText('question 0')).toBeInTheDocument()
    expect(screen.getAllByText('Filled cart').length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: /next page/i }))
    expect(await screen.findByText('an older question')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /previous page/i }))
    // the cached rows show at once and the refetch then redraws them, so assert on whatever is current
    await waitFor(() => expect(screen.getByText('question 0')).toBeInTheDocument())
  })

  it('starts a search from the first page', async () => {
    const first = Array.from({ length: 30 }, (_, i) => conversation(`c${i}`, `question ${i}`))
    renderPage({
      path: '/conversations',
      routes: { '/conversations': <ConversationsList /> },
      // typing takes the table back to its first page, which refetches while the search settles
      mocks: [
        ...signedIn,
        page(null, first, 'cursor-1'),
        page('cursor-1', [conversation('c30', 'an older question')], null),
        page(null, first, 'cursor-1'),
        page(null, [conversation('c7', 'shipping to Germany?')], null, 'ch1', 'germany'),
      ],
      scope: store,
      adminId: 'admin-1',
    })
    await screen.findByText('question 0')
    fireEvent.click(screen.getByRole('button', { name: /next page/i }))
    await screen.findByText('an older question')
    fireEvent.change(screen.getByPlaceholderText(/Search messages/), { target: { value: 'germany' } })
    expect(await screen.findByText('shipping to Germany?')).toBeInTheDocument()
  })

  it("starts another store's list at its own first page", async () => {
    const other = { ...store, scope: { ...store.scope, channelId: 'ch2' }, effectiveChannelId: 'ch2', selectedChannel: { id: 'ch2', name: 'Second' } } as typeof globalScope
    function Switchable() {
      const [scope, setScope] = useState(store)
      return (
        <ConfigScopeContext.Provider value={scope}>
          <button onClick={() => setScope(other)}>switch store</button>
          <ConversationsList />
        </ConfigScopeContext.Provider>
      )
    }
    const first = Array.from({ length: 30 }, (_, i) => conversation(`c${i}`, `question ${i}`))
    renderPage({
      path: '/conversations',
      routes: { '/conversations': <Switchable /> },
      mocks: [...signedIn, named('ch2'), page(null, first, 'cursor-1'), page('cursor-1', [conversation('c30', 'an older question')], null), page(null, [conversation('d1', 'from the second store')], null, 'ch2')],
      scope: store,
      adminId: 'admin-1',
    })
    await screen.findByText('question 0')
    fireEvent.click(screen.getByRole('button', { name: /next page/i }))
    await screen.findByText('an older question')
    fireEvent.click(screen.getByText('switch store'))
    expect(await screen.findByText('from the second store')).toBeInTheDocument()
  })
})

describe('ConversationsList filtered to unanswered', () => {
  const mars: Gap = { __typename: 'AssistantGap', turnId: 't9', question: 'Do you ship to Mars?', reasons: ['FOUND_NOTHING', 'UNSURE'] }

  it('narrows to conversations the store couldn\'t answer, says why, and opens one at that message', async () => {
    renderPage({
      path: '/conversations',
      routes: { '/conversations': <ConversationsList />, '/conversations/:id': <p>conversation open</p> },
      mocks: [
        ...signedIn,
        page(null, [conversation('c1', 'hello', 2, mars), conversation('c2', 'mugs?')], null),
        page(null, [conversation('c1', 'hello', 2, mars)], null, 'ch1', null, 'ANY'),
        page(null, [conversation('c1', 'hello', 1, { ...mars!, reasons: ['FOUND_NOTHING'] })], null, 'ch1', null, 'FOUND_NOTHING'),
      ],
      scope: store,
      adminId: 'admin-1',
    })
    expect(await screen.findByText('mugs?')).toBeInTheDocument()
    expect(screen.getByText(/^4 conversations had a message it couldn't answer/)).toBeInTheDocument()
    expect(screen.getByText('2 unanswered')).toBeInTheDocument()
    expect(screen.queryByText(/Couldn't answer/)).toBeNull()
    const filters = screen.getByRole('group', { name: 'Show' })
    expect(within(filters).getByRole('button', { name: /Found nothing\s*3/ })).toBeInTheDocument()

    fireEvent.click(within(filters).getByRole('button', { name: /Unanswered\s*4/ }))
    expect(await screen.findByText("Couldn't answer: Do you ship to Mars?")).toBeInTheDocument()
    expect(screen.queryByText('mugs?')).toBeNull()
    expect(within(screen.getByRole('table')).getByText("Didn't know")).toBeInTheDocument()

    fireEvent.click(within(filters).getByRole('button', { name: /Found nothing/ }))
    await waitFor(() => expect(within(screen.getByRole('table')).queryByText("Didn't know")).toBeNull())
    expect(within(filters).getByRole('button', { name: /Found nothing/ })).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(await screen.findByText("Couldn't answer: Do you ship to Mars?"))
    expect(await screen.findByText('conversation open')).toBeInTheDocument()
  })

  it('opens filtered when the address asks for it', async () => {
    renderPage({
      path: '/conversations?unanswered=no_answer',
      routes: { '/conversations': <ConversationsList /> },
      mocks: [...signedIn, page(null, [conversation('c1', 'hello', 1, { ...mars!, reasons: ['NO_ANSWER'] })], null, 'ch1', null, 'NO_ANSWER')],
      scope: store,
      adminId: 'admin-1',
    })
    expect(await screen.findByText("Couldn't answer: Do you ship to Mars?")).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /No answer/ })).toHaveAttribute('aria-pressed', 'true')
  })
})

describe('ConversationDetail', () => {
  const base = {
    __typename: 'AssistantTranscriptTurn',
    page: '/shop/bags',
    products: 0,
    cartChanged: false,
    checkout: false,
    incomplete: false,
    model: 'claude-opus-5-5',
    inputTokens: 100,
    outputTokens: 40,
    cacheReadTokens: 900,
    cacheWriteTokens: 0,
    at: '2026-11-04T10:00:00Z',
    refused: false,
  }
  const conversation = {
    __typename: 'AssistantConversation',
    id: 'conv-1',
    customerId: null,
    anonId: 'visitor-1234567',
    firstQuestion: 'Does it ship to Germany?',
    turns: 2,
    cartChanged: false,
    checkout: false,
    incomplete: false,
    startedAt: '2026-11-04T10:00:00Z',
    lastAt: '2026-11-04T10:00:00Z',
  }

  it('lays out the thread, flags a message that got no answer, and reads the steps when opened', async () => {
    const answered = {
      ...base,
      id: 't1',
      question: 'Does it ship to Germany?',
      reply: "Sorry, we don't ship to Germany.",
      recap: 'Explained in a panel: Canvas Backpack (id a930ea95-3020-4c0f-bba5-72675f32e576)',
      showed: [{ __typename: 'AssistantShownItem', kind: 'PANEL', productId: 'a930ea95-3020-4c0f-bba5-72675f32e576', name: 'Canvas Backpack', label: null, path: null, text: null }],
      followUps: ['Other countries?', 'Show me bags'],
      steps: 2,
      toolCalls: 2,
      thought: true,
    }
    const silent = { ...base, id: 't2', question: 'And to Austria?', reply: '', recap: '', showed: [], followUps: [], steps: 1, toolCalls: 0, thought: false }
    renderPage({
      path: '/conversations/conv-1',
      routes: { '/conversations/:id': <ConversationDetail /> },
      mocks: [
        ...signedIn,
        {
          request: { query: AssistantConversationDocument, variables: { channelId: 'ch1', id: 'conv-1' } },
          result: { data: { assistantConversation: { __typename: 'AssistantConversationDetail', conversation, turns: [answered, silent] } } },
        },
        {
          request: { query: AssistantTurnRoundsDocument, variables: { channelId: 'ch1', turnId: 't1' } },
          result: {
            data: {
              assistantTurnRounds: [
                {
                  __typename: 'AssistantRound',
                  thinking: 'Check delivery before recommending.',
                  text: "Sorry, we don't ship to Germany.",
                  shown: true,
                  calls: [{ __typename: 'AssistantToolCall', tool: 'estimate_shipping', input: '{"country":"DE"}', output: "The store doesn't deliver to DE.", isError: false, skipped: false }],
                },
                {
                  __typename: 'AssistantRound',
                  thinking: '',
                  text: '[[next: Other countries? | Show me bags]]',
                  shown: false,
                  calls: [{ __typename: 'AssistantToolCall', tool: 'show_products', input: '{}', output: '', isError: false, skipped: true }],
                },
              ],
            },
          },
        },
      ],
      scope: store,
    })
    expect(await screen.findAllByText('Does it ship to Germany?')).not.toHaveLength(0)
    expect(await screen.findAllByText('Ava')).toHaveLength(2)
    expect(screen.getByRole('link', { name: 'Canvas Backpack' })).toHaveAttribute('href', '/products/a930ea95-3020-4c0f-bba5-72675f32e576')
    expect(screen.getByText('Show me bags')).toBeInTheDocument()
    expect(screen.getAllByText(/Guest visitor visitor-/).length).toBeGreaterThan(0)
    expect(screen.getByText(/The shopper got no answer here/)).toBeInTheDocument()
    expect(screen.getByText(/1 message the shopper got no answer to/)).toBeInTheDocument()
    expect(screen.getByText(/2 steps · 2 tool calls · with thinking/)).toBeInTheDocument()

    fireEvent.click(screen.getAllByText('How it got there')[0])
    expect(await screen.findByText('Check delivery before recommending.')).toBeInTheDocument()
    expect(screen.getByText('Sent to the shopper')).toBeInTheDocument()
    expect(screen.queryByText(/not shown to the shopper/)).not.toBeInTheDocument()
    expect(screen.getByText(/Never ran/)).toBeInTheDocument()
    expect(screen.getByText('country: "DE"')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Priced delivery'))
    expect(screen.getByText(/"country": "DE"/)).toBeInTheDocument()
  })

  it('marks the message it was opened at', async () => {
    const one = { ...base, id: 't9', question: 'Unicorn saddles?', reply: 'None, sorry.', recap: '', showed: [], followUps: [], steps: 1, toolCalls: 1, thought: false }
    renderPage({
      path: '/conversations/conv-1#turn-t9',
      routes: { '/conversations/:id': <ConversationDetail /> },
      mocks: [
        ...signedIn,
        {
          request: { query: AssistantConversationDocument, variables: { channelId: 'ch1', id: 'conv-1' } },
          result: { data: { assistantConversation: { __typename: 'AssistantConversationDetail', conversation: { ...conversation, turns: 1 }, turns: [one] } } },
        },
      ],
      scope: store,
    })
    expect(await screen.findAllByText('Unicorn saddles?')).not.toHaveLength(0)
    expect(document.getElementById('turn-t9')).toHaveClass('ring-2')
  })

  it('says so when the store holds no such conversation', async () => {
    renderPage({
      path: '/conversations/gone',
      routes: { '/conversations/:id': <ConversationDetail /> },
      mocks: [...signedIn, { request: { query: AssistantConversationDocument, variables: { channelId: 'ch1', id: 'gone' } }, result: { data: { assistantConversation: null } } }],
      scope: store,
    })
    expect(await screen.findByText(/holds no such conversation/)).toBeInTheDocument()
  })
})
