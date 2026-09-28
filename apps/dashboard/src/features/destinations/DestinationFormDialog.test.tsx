import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog, stubResizeObserver } from '@/test/page-harness'
import { DestinationFormDialog } from './DestinationFormDialog'
import { CreateEventDestinationDocument, UpdateEventDestinationDocument } from './Destinations.gql'

stubResizeObserver()

describe('DestinationFormDialog', () => {
  it('needs a full URL and, for a new webhook, a signing secret', async () => {
    const onCreate = vi.fn()
    const onSaved = vi.fn()
    renderDialog(<DestinationFormDialog open onOpenChange={() => {}} channelId="c1" onSaved={onSaved} />, [
      {
        request: { query: CreateEventDestinationDocument, variables: captureInput(onCreate) },
        result: { data: { createEventDestination: { __typename: 'EventDestination', id: 'd1' } } },
      },
    ])
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Prod hook' } })
    fireEvent.change(screen.getByLabelText('Target URL'), { target: { value: 'example.com/hook' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create destination' }))
    expect(await screen.findByText('Enter a full URL, starting with https://')).toBeDefined()
    expect(screen.getByText('A new webhook destination needs a signing secret')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Target URL'), { target: { value: 'https://example.com/hook' } })
    fireEvent.change(screen.getByLabelText('Signing secret'), { target: { value: 'k3y' } })
    fireEvent.click(screen.getByLabelText('web'))
    fireEvent.click(screen.getByRole('button', { name: 'Create destination' }))
    await vi.waitFor(() => expect(onSaved).toHaveBeenCalled(), SLOW)
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ channelId: 'c1', kind: 'webhook', secret: 'k3y', eventSources: ['web'], eventNames: [] }),
    )
  })

  it('keeps the stored secret when an edit leaves it blank', async () => {
    const onUpdate = vi.fn()
    const onSaved = vi.fn()
    const existing = {
      id: 'd1',
      name: 'Prod hook',
      kind: 'webhook',
      targetUrl: 'https://example.com/hook',
      eventSources: [],
      eventNames: ['order_placed'],
      isActive: true,
    }
    renderDialog(<DestinationFormDialog open onOpenChange={() => {}} channelId="c1" existing={existing} onSaved={onSaved} />, [
      {
        request: {
          query: UpdateEventDestinationDocument,
          variables: (vars: { id: string; input: Record<string, unknown> }) => {
            onUpdate(vars)
            return true
          },
        },
        result: { data: { updateEventDestination: { __typename: 'EventDestination', id: 'd1' } } },
      },
    ])
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Prod webhook' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await vi.waitFor(() => expect(onSaved).toHaveBeenCalled(), SLOW)
    expect(onUpdate.mock.calls[0][0].id).toBe('d1')
    expect(onUpdate.mock.calls[0][0].input).toMatchObject({ name: 'Prod webhook', eventNames: ['order_placed'] })
    expect(onUpdate.mock.calls[0][0].input.secret).toBeUndefined()
  })
})
