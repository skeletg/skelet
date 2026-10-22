import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { globalScope, renderDialog } from '@/test/page-harness'
import { TranslationLocales } from './TranslationLocales'

// What this pins: Remove translation confirms through the shared ConfirmDialog - a refused remove keeps it open
// with the reason, a successful one closes it

async function confirmRemove(onRemove: () => Promise<void>) {
  renderDialog(<TranslationLocales locale="de" locales={['en', 'de']} entityLabel="post" canManage onRemove={onRemove} />, [], { ...globalScope, effectiveLocale: 'de' })
  fireEvent.click(screen.getByRole('button', { name: 'Remove translation' }))
  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText('Remove the de translation?')).toBeDefined()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Remove translation' }))
  return dialog
}

describe('TranslationLocales remove', () => {
  it('keeps the dialog open and shows a refused remove', async () => {
    const dialog = await confirmRemove(() => Promise.reject(new Error('default locale is locked')))
    expect(await within(dialog).findByText('default locale is locked')).toBeDefined()
    expect(screen.getByRole('alertdialog')).toBe(dialog)
  })

  it('closes once the remove goes through', async () => {
    await confirmRemove(async () => {})
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
  })
})
