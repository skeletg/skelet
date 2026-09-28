// Example Lexical block plugin - a coloured callout banner, showing the minimum surface a
// 3rd-party plugin needs: a DecoratorNode round-tripping via importJSON/exportJSON, a toolbar
// entry calling $insertNodes during editor.update(), and an optional plainText extractor so the
// body reaches description previews
// Location matters - loader.ts auto-imports src/plugins/<name>/plugin.{ts,tsx}, no core wiring

import { type ReactNode } from 'react'
import {
  $createParagraphNode,
  $getRoot,
  $getSelection,
  $insertNodes,
  $isRangeSelection,
  DecoratorNode,
  type LexicalEditor,
  type NodeKey,
} from 'lexical'
import { MegaphoneIcon } from 'lucide-react'

import { registerLexicalBlock } from '@/features/catalog/lexical-blocks'

type CalloutTone = 'info' | 'warning' | 'success'

type SerializedCalloutNode = {
  body: string
  tone: CalloutTone
  type: 'example-callout'
  version: 1
}

const TONE_CLASS: Record<CalloutTone, string> = {
  info: 'border-info/30 bg-info-soft text-info',
  warning: 'border-warning/30 bg-warning-soft text-warning',
  success: 'border-success/30 bg-success-soft text-success',
}

class CalloutNode extends DecoratorNode<ReactNode> {
  __body: string
  __tone: CalloutTone

  static getType(): string {
    return 'example-callout'
  }

  static clone(node: CalloutNode): CalloutNode {
    return new CalloutNode(node.__body, node.__tone, node.__key)
  }

  static importJSON(serialized: SerializedCalloutNode): CalloutNode {
    return new CalloutNode(serialized.body, serialized.tone)
  }

  constructor(body: string, tone: CalloutTone, key?: NodeKey) {
    super(key)
    this.__body = body
    this.__tone = tone
  }

  exportJSON(): SerializedCalloutNode {
    return { body: this.__body, tone: this.__tone, type: 'example-callout', version: 1 }
  }

  createDOM(): HTMLElement {
    const el = document.createElement('div')
    el.className = 'my-3'
    return el
  }

  updateDOM(): false {
    return false
  }

  decorate(): ReactNode {
    return (
      <aside className={`my-3 rounded-md border px-3 py-2 text-sm ${TONE_CLASS[this.__tone]}`}>
        <p className="font-medium">Callout</p>
        <p className="mt-1 whitespace-pre-wrap">{this.__body}</p>
      </aside>
    )
  }
}

function insertCallout(editor: LexicalEditor) {
  editor.update(() => {
    const selection = $getSelection()
    const node = new CalloutNode('Edit this callout in the JSON for now.', 'info')
    const trailing = $createParagraphNode()
    if ($isRangeSelection(selection)) {
      $insertNodes([node, trailing])
      return
    }
    $getRoot().append(node, trailing)
  })
}

registerLexicalBlock({
  node: CalloutNode,
  toolbar: {
    id: 'example-callout:default',
    label: 'Callout',
    section: 'Plugins',
    icon: MegaphoneIcon,
    insert: insertCallout,
  },
  extractPlainText: {
    nodeType: 'example-callout',
    extract: (node) => {
      const body = (node as { body?: unknown }).body
      return typeof body === 'string' ? body : ''
    },
  },
})
