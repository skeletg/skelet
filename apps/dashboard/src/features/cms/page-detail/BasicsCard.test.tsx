import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import type { CmsPageRowFragment } from '../Cms.gql'
import { BasicsCard } from './BasicsCard'

// What these pin: the basics review through the page's Save, a slug the backend would refuse is
// caught first, and the slug is only sent when it changed (a changed one creates a redirect)

const page = { id: 'pg1', slug: 'about', seoNoindex: false, data: { tagline: 'Hi' } } as unknown as CmsPageRowFragment

describe('BasicsCard', () => {
  it('refuses a slug with spaces, then saves', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderSection(<BasicsCard page={page} scalarAttrs={[]} canManage onSave={onSave} />)

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'About Us' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Lowercase letters, numbers/)).toBeDefined()
    expect(onSave).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'about-us' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith({ id: 'pg1', slug: 'about-us', seoNoindex: false, data: { tagline: 'Hi' } }), SLOW)
  })

  it('leaves the slug out when only noindex changed', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderSection(<BasicsCard page={page} scalarAttrs={[]} canManage onSave={onSave} />)
    fireEvent.click(screen.getByRole('switch'))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ slug: null, seoNoindex: true })), SLOW)
  })
})
