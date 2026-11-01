import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import type { CmsPageRowFragment } from '../Cms.gql'
import { BasicsCard } from './BasicsCard'
import type { PageTypeAttr } from './shared'

// What these pin: the basics review through the page's Save, a slug the backend would refuse is
// caught first, the slug is only sent when it changed (a changed one creates a redirect), and search
// indexing is never sent from here - the SEO panel owns it

const page = { id: 'pg1', slug: 'about', seoNoindex: true, data: { tagline: 'Hi' } } as unknown as CmsPageRowFragment
const tagline = {
  isRequired: false,
  attribute: { id: 'a1', slug: 'tagline', inputType: 'TEXT', isTranslatable: false, isRequiredDefault: false, content: { name: 'Tagline' }, values: [] },
} as unknown as PageTypeAttr

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
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith({ id: 'pg1', slug: 'about-us', data: { tagline: 'Hi' } }), SLOW)
  })

  it('leaves the slug and search indexing out when only a field changed', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderSection(<BasicsCard page={page} scalarAttrs={[tagline]} canManage onSave={onSave} />)
    expect(screen.queryByRole('switch')).toBeNull()
    fireEvent.change(screen.getByLabelText('Tagline'), { target: { value: 'Hello' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith({ id: 'pg1', slug: null, data: { tagline: 'Hello' } }), SLOW)
  })
})
