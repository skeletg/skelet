import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { ServerFeaturesDocument } from '@/shared/features/Features.gql'
import { ChatDock, ChatProvider, ChatTrigger } from './ChatPanel'
import { AiChatStatusDocument, ChatConversationsDocument, DeleteChatConversationDocument } from './Ai.gql'

// What this pins: the history list's delete is a one-hover click that can't be undone, so it asks first

// jsdom has no element scrolling; the dock scrolls its transcript to the bottom on every render
Element.prototype.scrollTo ??= () => {}

describe('ChatDock delete chat', () => {
  it('deletes a conversation only after the admin confirms', async () => {
    const removed = vi.fn()
    renderPage({
      path: '/',
      routes: {
        '/': (
          <ChatProvider>
            <ChatTrigger />
            <ChatDock />
          </ChatProvider>
        ),
      },
      mocks: [
        meMock(),
        {
          request: { query: ServerFeaturesDocument },
          result: { data: { serverFeatures: { __typename: 'ServerFeatures', signals: true, ai: true } } },
          maxUsageCount: 5,
        },
        { request: { query: AiChatStatusDocument }, result: { data: { aiStatus: { __typename: 'AIStatus', enabled: true } } }, maxUsageCount: 5 },
        {
          request: { query: ChatConversationsDocument, variables: { limit: 20 } },
          result: { data: { chatConversations: [{ __typename: 'ChatConversation', id: 'c1', title: 'Weekly sales' }] } },
          maxUsageCount: 5,
        },
        { request: { query: DeleteChatConversationDocument, variables: captureInput(removed) }, result: { data: { deleteChatConversation: true } } },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Chat' }, SLOW))
    fireEvent.click(await screen.findByRole('button', { name: 'Chat history' }, SLOW))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete chat' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(removed).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete chat' }))
    await vi.waitFor(() => expect(removed).toHaveBeenCalledWith({ id: 'c1' }), SLOW)
  })
})
