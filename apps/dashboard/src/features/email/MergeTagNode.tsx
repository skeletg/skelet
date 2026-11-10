import type { ReactNode } from 'react'
import { DecoratorNode, type NodeKey, type SerializedLexicalNode, type Spread, type TextFormatType } from 'lexical'
import { mergeTagLabel } from './blocks'

export const MERGE_TAG_FORMATS = ['bold', 'italic', 'strikethrough', 'underline'] as const satisfies readonly TextFormatType[]
type MergeTagFormat = typeof MERGE_TAG_FORMATS[number]
type SerializedMergeTag = Spread<{ tag: string; formats?: MergeTagFormat[] }, SerializedLexicalNode>
const FORMAT_TAGS: Record<MergeTagFormat, string> = { bold: 'strong', italic: 'em', strikethrough: 's', underline: 'u' }

// A merge tag in the text - one unit the caret steps over, shown by name, saved as {{tag}}
export class MergeTagNode extends DecoratorNode<ReactNode> {
  __tag: string
  __formats: MergeTagFormat[]

  static getType(): string {
    return 'merge-tag'
  }

  static clone(node: MergeTagNode): MergeTagNode {
    return new MergeTagNode(node.__tag, node.__formats, node.__key)
  }

  static importJSON(json: SerializedMergeTag): MergeTagNode {
    return $createMergeTagNode(json.tag, json.formats)
  }

  constructor(tag: string, formats: MergeTagFormat[] = [], key?: NodeKey) {
    super(key)
    this.__tag = tag
    this.__formats = [...formats]
  }

  exportJSON(): SerializedMergeTag {
    return { ...super.exportJSON(), type: 'merge-tag', version: 1, tag: this.__tag, formats: [...this.__formats] }
  }

  createDOM(): HTMLElement {
    return document.createElement('span')
  }

  updateDOM(): false {
    return false
  }

  exportDOM() {
    let element: Text | HTMLElement = document.createTextNode(`{{${this.__tag}}}`)
    for (const format of MERGE_TAG_FORMATS) {
      if (!this.__formats.includes(format)) continue
      const wrapper = document.createElement(FORMAT_TAGS[format])
      wrapper.append(element)
      element = wrapper
    }
    return { element }
  }

  getTextContent(): string {
    return `{{${this.__tag}}}`
  }

  isInline(): true {
    return true
  }

  decorate(): ReactNode {
    return (
      <span
        data-merge-tag={this.__tag}
        className="rounded bg-info-soft px-1 text-info"
        title={`{{${this.__tag}}}`}
        style={{
          fontWeight: this.__formats.includes('bold') ? 700 : undefined,
          fontStyle: this.__formats.includes('italic') ? 'italic' : undefined,
          textDecorationLine: [this.__formats.includes('underline') && 'underline', this.__formats.includes('strikethrough') && 'line-through'].filter(Boolean).join(' ') || undefined,
        }}
      >
        {mergeTagLabel(this.__tag)}
      </span>
    )
  }
}

export function $createMergeTagNode(tag: string, formats: MergeTagFormat[] = []): MergeTagNode {
  return new MergeTagNode(tag, formats)
}
