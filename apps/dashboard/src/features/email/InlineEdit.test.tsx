import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { $createTextNode, $getRoot, $isElementNode, type LexicalEditor } from 'lexical'
import { InlineEditor } from './InlineEdit'
import type { EmailBlock } from './blocks'

// What these pin: the words reopen when they change underneath, and a plain block stays plain until it's edited
function editorOf(): LexicalEditor {
  return (screen.getByRole('textbox', { name: 'Text' }) as HTMLElement & { __lexicalEditor: LexicalEditor }).__lexicalEditor
}

describe('the inline text editor', () => {
  it('opens afresh when its words change under it', async () => {
    const block: EmailBlock = { type: 'text', format: 'rich', content: '<p>One</p>' }
    const { rerender } = render(<InlineEditor block={block} theme={{}} tags={[]} onChange={vi.fn()} />)
    expect(await screen.findByText('One')).toBeDefined()
    rerender(<InlineEditor block={{ ...block, content: '<p>Two</p>' }} theme={{}} tags={[]} onChange={vi.fn()} />)
    expect(await screen.findByText('Two')).toBeDefined()
  })

  it('leaves a plain block alone until it is edited', async () => {
    const onChange = vi.fn()
    render(<InlineEditor block={{ type: 'text', content: 'Hi <b>there</b>' }} theme={{}} tags={[]} onChange={onChange} />)
    expect(await screen.findByText('Hi <b>there</b>')).toBeDefined()
    expect(onChange).not.toHaveBeenCalled()
    await act(async () => {
      editorOf().update(() => {
        const first = $getRoot().getFirstChild()
        if ($isElementNode(first)) first.append($createTextNode('!'))
      })
    })
    expect(onChange).toHaveBeenLastCalledWith({ type: 'text', format: 'rich', content: '<p>Hi &lt;b&gt;there&lt;/b&gt;!</p>' })
  })

  it.each([
    '<strong>{{order.number}}</strong>',
    '<em>{{order.number}}</em>',
    '<u>{{order.number}}</u>',
    '<s>{{order.number}}</s>',
    '<u><s><em><strong>{{order.number}}</strong></em></s></u>',
  ])('preserves formatted merge tags after editing and reopening: %s', async (tag) => {
    const onChange = vi.fn()
    const block: EmailBlock = { type: 'text', format: 'rich', content: `<p>Order ${tag}</p>` }
    const props = { theme: {}, tags: [], onChange }
    const { rerender } = render(<InlineEditor block={block} {...props} />)
    expect(await screen.findByText('Order number')).toBeDefined()
    expect(onChange).not.toHaveBeenCalled()
    await act(async () => {
      editorOf().update(() => {
        const first = $getRoot().getFirstChild()
        if ($isElementNode(first)) first.append($createTextNode('!'))
      })
    })
    expect(onChange).toHaveBeenLastCalledWith({ ...block, content: `<p>Order ${tag}!</p>` })
    rerender(<InlineEditor key="reopened" block={onChange.mock.lastCall![0]} {...props} />)
    expect(await screen.findByText('Order number')).toBeDefined()
    await act(async () => {
      editorOf().update(() => {
        const first = $getRoot().getFirstChild()
        if ($isElementNode(first)) first.append($createTextNode('?'))
      })
    })
    expect(onChange).toHaveBeenLastCalledWith({ ...block, content: `<p>Order ${tag}!?</p>` })
  })
})
