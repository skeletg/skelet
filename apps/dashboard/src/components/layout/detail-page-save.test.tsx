import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { Button } from '@/components/ui/button'
import { DetailPageHeader } from './detail-page'

// What this pins: ⌘S / Ctrl+S presses the page's Save - and nothing else. The browser's save-page dialog never opens,
// a disabled Save stays unpressed, and a second press while the review dialog is up doesn't stack another

function page(onSave: () => void, opts: { disabled?: boolean; dialog?: boolean } = {}) {
  render(
    <MemoryRouter>
      <DetailPageHeader title="Group" backTo={null} actions={<Button disabled={opts.disabled} onClick={onSave}>Save changes</Button>} />
      {opts.dialog && <div role="alertdialog">Review</div>}
    </MemoryRouter>,
  )
}

const press = (init: KeyboardEventInit) => fireEvent.keyDown(window, { key: 's', ...init })

describe('DetailPageHeader save shortcut', () => {
  it('presses Save on ⌘S and on Ctrl+S, instead of the browser saving the page', () => {
    const onSave = vi.fn()
    page(onSave)
    expect(press({ metaKey: true })).toBe(false) // default prevented
    press({ ctrlKey: true })
    expect(onSave).toHaveBeenCalledTimes(2)
  })

  it('does nothing while Save is disabled', () => {
    const onSave = vi.fn()
    page(onSave, { disabled: true })
    expect(press({ metaKey: true })).toBe(false)
    expect(onSave).not.toHaveBeenCalled()
  })

  it('leaves a press under an open dialog alone', () => {
    const onSave = vi.fn()
    page(onSave, { dialog: true })
    press({ metaKey: true })
    expect(onSave).not.toHaveBeenCalled()
  })

  it('ignores a plain S and other modifier combinations', () => {
    const onSave = vi.fn()
    page(onSave)
    press({})
    press({ metaKey: true, shiftKey: true })
    expect(onSave).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDefined()
  })
})
