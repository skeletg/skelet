import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { FormRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'

// What this pins: a value put into the form from outside (a restored draft) shows in the editor,
// which on its own only reads its value when it mounts

function Host() {
  const [value, setValue] = useState<RichTextValue>('Stored copy')
  return (
    <>
      <button type="button" onClick={() => setValue('Restored copy')}>
        Restore
      </button>
      <FormRichTextEditor value={value} fallbackText="" onChange={(next) => setValue(next)} />
    </>
  )
}

describe('FormRichTextEditor', () => {
  it('shows a value set from outside the editor', async () => {
    render(<Host />)
    expect(await screen.findByText('Stored copy')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Restore' }))
    expect(await screen.findByText('Restored copy')).toBeDefined()
  })
})
