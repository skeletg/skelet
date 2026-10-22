/* eslint-disable react-refresh/only-export-components */
// FAQ block - operator-curated list of Q/A pairs - Self-registers
// both a node class and a "FAQ" entry in the Custom Blocks dropdown
//
// SYNC: The SerializedFAQBlockNode shape below is mirrored in
// apps/channels/vetrina-demo/app/lib/lexical-render.tsx (FAQBlock type)
// Renaming a field here without updating the channel breaks blog
// post rendering

import { useEffect, useState, type ReactNode } from 'react'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import {
  $createParagraphNode,
  $getNodeByKey,
  $getRoot,
  $getSelection,
  $insertNodes,
  $isRangeSelection,
  DecoratorNode,
  type LexicalEditor,
  type LexicalNode,
  type NodeKey,
} from 'lexical'
import { PlusIcon, Trash2Icon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import { registerLexicalBlock } from './registry'

type FAQItem = {
  answer: string
  question: string
}

type SerializedFAQBlockNode = {
  items: FAQItem[]
  type: 'faq-block'
  version: 1
}

function defaultFAQItems(): FAQItem[] {
  return [
    { question: 'What should customers know?', answer: 'Add the first answer here.' },
    { question: 'How does shipping work?', answer: 'Add shipping details here.' },
  ]
}

class FAQBlockNode extends DecoratorNode<ReactNode> {
  __items: FAQItem[]

  static getType(): string {
    return 'faq-block'
  }

  static clone(node: FAQBlockNode): FAQBlockNode {
    return new FAQBlockNode(node.__items, node.__key)
  }

  static importJSON(serializedNode: SerializedFAQBlockNode): FAQBlockNode {
    return new FAQBlockNode(serializedNode.items)
  }

  constructor(items: FAQItem[], key?: NodeKey) {
    super(key)
    this.__items = items
  }

  exportJSON(): SerializedFAQBlockNode {
    return {
      items: this.__items,
      type: 'faq-block',
      version: 1,
    }
  }

  // isInline=false and isTopLevel=true together tell Lexical's
  // $insertNodes to lift this node out of any containing paragraph
  // when inserting - otherwise it nests inside the cursor's
  // paragraph, which the channel renderer has to clean up at read
  // time - Block-level by nature: FAQs are their own section
  isInline(): false {
    return false
  }

  isTopLevel(): true {
    return true
  }

  createDOM(): HTMLElement {
    const element = document.createElement('div')
    element.className = 'my-4'
    return element
  }

  updateDOM(): false {
    return false
  }

  getItems(): FAQItem[] {
    return this.getLatest().__items
  }

  setItems(items: FAQItem[]) {
    const writable = this.getWritable()
    writable.__items = items
  }

  decorate(): ReactNode {
    return <FAQBlockComponent nodeKey={this.__key} items={this.__items} />
  }
}

function $isFAQBlockNode(node: LexicalNode | null | undefined): node is FAQBlockNode {
  return node instanceof FAQBlockNode
}

function FAQBlockComponent({ nodeKey, items }: { nodeKey: NodeKey; items: FAQItem[] }) {
  const [editor] = useLexicalComposerContext()
  const [editable, setEditable] = useState(() => editor.isEditable())

  useEffect(() => editor.registerEditableListener(setEditable), [editor])

  function updateItems(nextItems: FAQItem[]) {
    editor.update(() => {
      const node = $getNodeByKey(nodeKey)
      if ($isFAQBlockNode(node)) {
        node.setItems(nextItems)
      }
    })
  }

  function updateItem(index: number, patch: Partial<FAQItem>) {
    updateItems(items.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)))
  }

  function addItem() {
    updateItems([...items, { question: '', answer: '' }])
  }

  function removeItem(index: number) {
    updateItems(items.filter((_, itemIndex) => itemIndex !== index))
  }

  if (!editable) {
    return (
      <section className="my-4 rounded-lg border bg-muted/20 p-4">
        <p className="mb-3 text-sm font-semibold">FAQ</p>
        <div className="grid gap-3">
          {items.map((item, index) => (
            <div key={index} className="rounded-md border bg-background p-3">
              <p className="text-sm font-medium">{item.question || 'Untitled question'}</p>
              <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{item.answer || 'No answer yet.'}</p>
            </div>
          ))}
        </div>
      </section>
    )
  }

  return (
    <section className="my-4 rounded-lg border bg-muted/20 p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-semibold">FAQ block</p>
          <p className="text-xs text-muted-foreground">Add reusable questions and answers for this description.</p>
        </div>
        <Button type="button" size="sm" variant="outline" onClick={addItem}>
          <PlusIcon className="size-4" />
          Add item
        </Button>
      </div>
      <div className="grid gap-3">
        {items.map((item, index) => (
          <div key={index} className="grid gap-2 rounded-md border bg-background p-3">
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Question {index + 1}</span>
              <Button aria-label={`Remove question ${index + 1}`} type="button" size="icon" variant="ghost" className="size-8" onClick={() => removeItem(index)}>
                <Trash2Icon className="size-4" />
              </Button>
            </div>
            <Input
              value={item.question}
              onChange={(event) => updateItem(index, { question: event.target.value })}
              placeholder="Question"
            />
            <Textarea
              value={item.answer}
              onChange={(event) => updateItem(index, { answer: event.target.value })}
              placeholder="Answer"
              rows={4}
            />
          </div>
        ))}
      </div>
    </section>
  )
}

function insertFAQBlock(editor: LexicalEditor) {
  editor.update(() => {
    const selection = $getSelection()
    const faqNode = new FAQBlockNode(defaultFAQItems())
    const trailingParagraph = $createParagraphNode()

    if ($isRangeSelection(selection)) {
      $insertNodes([faqNode, trailingParagraph])
      return
    }
    $getRoot().append(faqNode, trailingParagraph)
  })
}

registerLexicalBlock({
  node: FAQBlockNode,
  toolbar: {
    id: 'core:faq',
    label: 'FAQ',
    section: 'Content',
    insert: insertFAQBlock,
  },
  extractPlainText: {
    nodeType: 'faq-block',
    extract: (node) => {
      const maybeItems = (node as { items?: unknown }).items
      if (!Array.isArray(maybeItems)) return ''
      return maybeItems
        .map((item) => {
          if (!item || typeof item !== 'object') return ''
          const question = typeof (item as { question?: unknown }).question === 'string' ? (item as { question: string }).question : ''
          const answer = typeof (item as { answer?: unknown }).answer === 'string' ? (item as { answer: string }).answer : ''
          return [question, answer].filter(Boolean).join(' ')
        })
        .filter(Boolean)
        .join(' ')
    },
  },
})
