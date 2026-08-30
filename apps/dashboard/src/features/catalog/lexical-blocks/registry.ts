// Lexical block registry - the rich-text extension point, mirroring the nav registry: side-effect imports populate the arrays, the editor
// reads them at mount Append-only with no unregister, so Vite HMR re-running the side effects is harmless

import type { LexicalEditor, LexicalNode } from 'lexical'
import type { ComponentType } from 'react'

type NodeKlass = new (...args: never[]) => LexicalNode  // the shape LexicalComposer expects in its `nodes` array

export type LexicalBlockToolbarEntry = {
  id: string  // "<plugin-slug>:<block-slug>", or "core:<slug>" for built-ins
  label: string
  section?: string  // entries sharing a section render under one heading, in registration order
  icon?: ComponentType<{ className?: string }>
  insert: (editor: LexicalEditor) => void  // typically wraps an editor.update() building the node plus a trailing paragraph
}

export type LexicalBlockDefinition = {
  node?: NodeKlass  // the registry dedupes by class identity, so variants sharing a node class declare it once
  toolbar?: Omit<LexicalBlockToolbarEntry, 'id'> & { id: string }
  extractPlainText?: {  // without this the block contributes nothing to description previews
    nodeType: string
    extract: (node: unknown) => string
  }
}

const nodes = new Set<NodeKlass>()
const toolbarEntries: LexicalBlockToolbarEntry[] = []
const extractors = new Map<string, (node: unknown) => string>()

export function registerLexicalBlock(def: LexicalBlockDefinition): void {  // same node class twice is a no-op (Set), same toolbar id throws - two identical menu items surprise more than a boot error
  if (def.node) {
    nodes.add(def.node)
  }
  if (def.toolbar) {
    if (toolbarEntries.some((entry) => entry.id === def.toolbar!.id)) {
      throw new Error(`lexical-blocks: duplicate toolbar id "${def.toolbar.id}"`)
    }
    toolbarEntries.push(def.toolbar)
  }
  if (def.extractPlainText) {
    extractors.set(def.extractPlainText.nodeType, def.extractPlainText.extract)
  }
}

export function getLexicalBlockNodes(): NodeKlass[] {  // registration order is fine - Lexical dispatches on getType(), not array position
  return Array.from(nodes)
}

export function getLexicalBlockToolbarEntries(): LexicalBlockToolbarEntry[] {  // the editor groups by section when rendering - that grouping is a UI concern, kept out of here
  return toolbarEntries.slice()
}

export function extractPlainTextFor(  // undefined when no block claims the type, so richTextToPlainText falls back to descending into children
  nodeType: string,
): ((node: unknown) => string) | undefined {
  return extractors.get(nodeType)
}

export function __resetForTests(): void {  // unit tests only, never production
  nodes.clear()
  toolbarEntries.length = 0
  extractors.clear()
}
