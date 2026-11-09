import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { $createTextNode, $getRoot, $isElementNode, FORMAT_TEXT_COMMAND, type LexicalEditor } from 'lexical'
import { INSERT_UNORDERED_LIST_COMMAND } from '@lexical/list'
import { RichTextEditor } from './RichTextEditor'

// What these pin: merge tags open as chips, formatting saves as the subset, and opening alone saves nothing
function mount(html: string) {
  const onChange = vi.fn()
  render(<RichTextEditor html={html} onChange={onChange} tags={['customer.firstName', 'order.number']} />)
  const el = screen.getByRole('textbox', { name: 'Text' }) as HTMLElement & { __lexicalEditor: LexicalEditor }
  return { onChange, editor: el.__lexicalEditor }
}

describe('the email text editor', () => {
  it('opens on the stored text with merge tags as named chips, without saving anything', async () => {
    const { onChange } = mount('<p>Hi {{customer.firstName}}, <strong>welcome</strong></p>')
    expect(await screen.findByText('First name')).toBeDefined()
    expect(screen.getByText('welcome').closest('strong, .font-bold')).not.toBeNull()
    expect(onChange).not.toHaveBeenCalled()
  })

  it('saves bold and lists as the subset, merge tags as {{tags}}', async () => {
    const { onChange, editor } = mount('<p>Hi {{customer.firstName}}</p>')
    await act(async () => {
      editor.update(() => {
        const first = $getRoot().getFirstChild()
        if ($isElementNode(first)) first.select(0, first.getChildrenSize())
      })
    })
    await act(async () => {
      editor.dispatchCommand(FORMAT_TEXT_COMMAND, 'bold')
    })
    expect(onChange).toHaveBeenLastCalledWith('<p><strong>Hi </strong>{{customer.firstName}}</p>')
    await act(async () => {
      editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined)
    })
    expect(onChange).toHaveBeenLastCalledWith('<ul><li><strong>Hi </strong>{{customer.firstName}}</li></ul>')
  })

  it('turns typed {{tag}} text into a chip', async () => {
    const { onChange, editor } = mount('<p>Order</p>')
    expect(await screen.findByText('Order')).toBeDefined()
    await act(async () => {
      editor.update(() => {
        const first = $getRoot().getFirstChild()
        if ($isElementNode(first)) first.append($createTextNode(' {{order.number}} shipped'))
      })
    })
    expect(await screen.findByText('Order number')).toBeDefined()
    expect(onChange).toHaveBeenLastCalledWith('<p>Order {{order.number}} shipped</p>')
  })
})
