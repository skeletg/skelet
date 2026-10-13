import { act, renderHook } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { SectionRegistration } from '@/shared/saving/saveRegistry'
import type { Configuration } from './types'
import { CreateCheckoutOptionDocument, DeleteCheckoutOptionDocument, SaveCheckoutDeliveryPointsDocument, UpdateCheckoutOptionDocument } from './Checkout.gql'
const mocks = vi.hoisted(() => ({ mutate: vi.fn(), register: vi.fn() }))
vi.mock('@apollo/client/react', () => ({ useApolloClient: () => ({ mutate: mocks.mutate }) }))
vi.mock('@/shared/saving/saveRegistry', () => ({ useSaveSection: mocks.register }))
const { useCheckoutDraft } = await import('./useCheckoutDraft')
beforeEach(() => vi.clearAllMocks())
const config = { usingDefaults: false, options: [], deliveryPoints: [], kinds: [{ slug: 'parcel_locker', label: 'Pickup point', blocks: [] }], pointSources: [] } as unknown as Configuration
function latestSave() { return (mocks.register.mock.lastCall![0] as SectionRegistration).save() }
it('saves points before activation and retains their IDs and option IDs when retrying', async () => {
  let updates = 0
  const pointInputs: Array<Array<{ id: string | null }>> = []
  mocks.mutate.mockImplementation(async ({ mutation, variables }) => {
    if (mutation === SaveCheckoutDeliveryPointsDocument) { pointInputs.push(variables.points); return { data: { saveDeliveryPoints: [{ id: 'saved-point' }] } } }
    if (mutation === CreateCheckoutOptionDocument) return { data: { createDeliveryOption: { id: 'saved-option' } } }
    if (mutation === UpdateCheckoutOptionDocument && ++updates === 1) throw new Error('not ready')
    return { data: { updateDeliveryOption: { id: 'saved-option' } } }
  })
  const onSaved = vi.fn(async () => {})
  const { result } = renderHook(() => useCheckoutDraft({ channelId: 'channel', configuration: config, onSaved }))
  act(() => {
    const point = result.current.addPoint()
    result.current.patchPoint(point, { name: 'Central', street1: 'Main street', city: 'Belgrade', postalCode: '11000', countryCode: 'RS' })
    const option = result.current.addOption('parcel_locker')
    result.current.patchOption(option, { enabled: true })
  })
  await act(async () => { await expect(latestSave()).rejects.toThrow('not ready') })
  act(() => result.current.patchOption(result.current.options[0].id, { name: 'Central pickup' }))
  await act(async () => { await latestSave() })
  expect(pointInputs.map((points) => points[0].id)).toEqual([null, 'saved-point'])
  expect(mocks.mutate.mock.calls.filter(([args]) => args.mutation === CreateCheckoutOptionDocument)).toHaveLength(1)
  expect(mocks.mutate.mock.calls.map(([args]) => args.mutation)).toEqual([SaveCheckoutDeliveryPointsDocument, CreateCheckoutOptionDocument, UpdateCheckoutOptionDocument, SaveCheckoutDeliveryPointsDocument, UpdateCheckoutOptionDocument])
  expect(mocks.mutate.mock.lastCall![0].variables).toEqual(expect.objectContaining({ id: 'saved-option', input: expect.objectContaining({ name: 'Central pickup', enabled: true }) }))
  expect(onSaved).toHaveBeenCalledTimes(1)
})
it('can remove an option created during a failed save without leaving an orphan', async () => {
  mocks.mutate.mockImplementation(async ({ mutation }) => {
    if (mutation === CreateCheckoutOptionDocument) return { data: { createDeliveryOption: { id: 'saved-option' } } }
    if (mutation === UpdateCheckoutOptionDocument) throw new Error('not ready')
    return { data: { deleteDeliveryOption: true } }
  })
  const { result } = renderHook(() => useCheckoutDraft({ channelId: 'channel', configuration: config, onSaved: vi.fn(async () => {}) }))
  act(() => { const id = result.current.addOption('parcel_locker'); result.current.patchOption(id, { enabled: true }) })
  await act(async () => { await expect(latestSave()).rejects.toThrow('not ready') })
  act(() => result.current.deleteOption(result.current.options[0].id))
  expect((mocks.register.mock.lastCall![0] as SectionRegistration).diff).toEqual([expect.objectContaining({ kind: 'remove', label: 'Delivery · Pickup point' })])
  await act(async () => { await latestSave() })
  expect(mocks.mutate.mock.lastCall![0]).toEqual({ mutation: DeleteCheckoutOptionDocument, variables: { id: 'saved-option' } })
})
