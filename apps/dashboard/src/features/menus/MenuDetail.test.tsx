import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { MenuDetail } from './MenuDetail'
import {
  DeleteMenuItemTranslationDocument,
  MenuDetailDocument,
  SetMenuStoresDocument,
  UpdateMenuDocument,
  UpdateMenuItemDocument,
  UpsertMenuItemTranslationDocument,
} from './Menus.gql'

// What these pin: the menu's name and channels go through the one header Save (they had a Save
// button each), and an item's translations are written with the dialog's Save, not per keystroke

const item = {
  __typename: 'MenuItem' as const,
  id: 'i1',
  menuId: 'm1',
  parentId: null,
  label: 'Shop',
  targetType: null,
  targetId: null,
  url: '/shop',
  sortOrder: 0,
  translations: [
    { __typename: 'MenuItemTranslation' as const, locale: 'de', label: 'Laden' },
    { __typename: 'MenuItemTranslation' as const, locale: 'fr', label: 'Boutique' },
  ],
}

const menu = {
  __typename: 'Menu' as const,
  id: 'm1',
  slug: 'main-nav',
  name: 'Main nav',
  itemCount: 1,
  channelIds: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  items: [item],
}

const channel = (id: string, name: string) => ({
  __typename: 'Channel' as const,
  id,
  slug: name.toLowerCase(),
  name,
  defaultLocale: 'en',
  locales: [],
  isActive: true,
  isDefault: false,
  currencyCode: 'EUR',
  publicUrl: null,
  createdAt: '2026-01-01T00:00:00Z',
})

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderMenu() {
  const writes = { menu: vi.fn(), stores: vi.fn(), item: vi.fn(), upsert: vi.fn(), remove: vi.fn() }
  renderPage({
    path: '/menus/m1',
    adminId: 'admin-1',
    routes: { '/menus/:id': <MenuDetail /> },
    mocks: [
      meMock(),
      { request: { query: MenuDetailDocument, variables: { id: 'm1' } }, result: { data: { menu } }, maxUsageCount: 5 },
      { request: { query: ChannelsDocument }, result: { data: { channels: [channel('c1', 'Wheels')] } }, maxUsageCount: 3 },
      { request: { query: LocalesPickerDocument }, result: { data: { locales: [] } }, maxUsageCount: 3 },
      { request: { query: UpdateMenuDocument, variables: recorder(writes.menu) }, result: { data: { updateMenu: { ...menu, name: 'Header' } } } },
      { request: { query: SetMenuStoresDocument, variables: recorder(writes.stores) }, result: { data: { setMenuStores: { ...menu, channelIds: ['c1'] } } } },
      { request: { query: UpdateMenuItemDocument, variables: recorder(writes.item) }, result: { data: { updateMenuItem: item } } },
      { request: { query: UpsertMenuItemTranslationDocument, variables: recorder(writes.upsert) }, result: { data: { upsertMenuItemTranslation: item } } },
      { request: { query: DeleteMenuItemTranslationDocument, variables: recorder(writes.remove) }, result: { data: { deleteMenuItemTranslation: true } } },
    ],
  })
  return writes
}

afterEach(() => localStorage.clear())

describe('MenuDetail', () => {
  it('reviews the name and channels together, then writes both', async () => {
    const writes = renderMenu()
    fireEvent.change(await screen.findByLabelText('Name', {}, SLOW), { target: { value: 'Header' } })
    fireEvent.click(await screen.findByRole('checkbox', { name: /Wheels/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.stores).toHaveBeenCalledWith({ menuId: 'm1', channelIds: ['c1'] }), SLOW)
    expect(writes.menu).toHaveBeenCalledWith({ input: { id: 'm1', slug: 'main-nav', name: 'Header' } })
  })

  it('writes an item and its translation edits when the dialog saves', async () => {
    const writes = renderMenu()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }, SLOW))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Label (de)'), { target: { value: 'Geschäft' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove fr translation' }))
    expect(writes.upsert).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    await vi.waitFor(() => expect(writes.remove).toHaveBeenCalledWith({ itemId: 'i1', locale: 'fr' }), SLOW)
    expect(writes.item).toHaveBeenCalledWith({ input: expect.objectContaining({ id: 'i1', label: 'Shop', url: '/shop', clearParent: false }) })
    expect(writes.upsert).toHaveBeenCalledWith({ itemId: 'i1', locale: 'de', label: 'Geschäft' })
  })

  it('refuses a URL link without a URL', async () => {
    const writes = renderMenu()
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }, SLOW))
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('URL'), { target: { value: '' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(await within(dialog).findByText('URL is required')).toBeDefined()
    expect(writes.item).not.toHaveBeenCalled()
  })
})
