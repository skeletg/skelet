import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { MenuCreate } from './MenuCreate'
import { CreateMenuDocument } from './Menus.gql'

// What these pin: the slug follows the name until edited by hand, a slug the backend would refuse
// is caught before anything is sent, and a create lands on the new menu's page

describe('MenuCreate', () => {
  it('fills the slug from the name, refuses a bad one, then creates the menu', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/menus/new',
      routes: { '/menus/new': <MenuCreate />, '/menus/:id': <p>menu page</p> },
      mocks: [
        {
          request: { query: CreateMenuDocument, variables: captureInput(onCreate) },
          result: { data: { createMenu: { __typename: 'Menu', id: 'm1', slug: 'main-nav', name: 'Main Nav' } } },
        },
      ],
    })

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Main Nav' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('main-nav')

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'Main.Nav' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create menu' }))
    expect(await screen.findByText(/Lowercase letters, numbers/)).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'main-nav' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create menu' }))
    expect(await screen.findByText('menu page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith({ slug: 'main-nav', name: 'Main Nav' })
  })
})
