/* eslint-disable react-refresh/only-export-components */
// Structured block - one Lexical node that polymorphically renders
// seven variants (pros-cons, comparison-table, tabs, testimonials,
// video-embed, specs-table, image-text) - Each variant gets its own
// "Custom Blocks" dropdown entry; the registry dedupes the shared
// node class
//
// SYNC: The SerializedStructuredBlockNode + per-variant data shapes
// below are mirrored in packages/storefront-core/lexical.ts
// (StructuredBlock and the {ProsCons,ComparisonTable,Tabs,
// Testimonials,VideoEmbed,SpecsTable,ImageText}Data types)
// Renaming a field or adding a new blockType variant here without
// updating the channel breaks blog post rendering - unknown
// variants render as empty (warning logged in dev)

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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

import { registerLexicalBlock } from './registry'

type StructuredBlockType =
  | 'pros-cons'
  | 'comparison-table'
  | 'tabs'
  | 'testimonials'
  | 'video-embed'
  | 'specs-table'
  | 'image-text'

type ComparisonTableData = {
  columns: string[]
  rows: string[][]
}

type ImageTextData = {
  body: string
  imageAlt: string
  imageUrl: string
  layout: 'image-left' | 'image-right'
  title: string
}

type KeyValueRow = {
  label: string
  value: string
}

type ProsConsData = {
  cons: string[]
  pros: string[]
}

type TabItem = {
  content: string
  label: string
}

type TabsBlockData = {
  tabs: TabItem[]
}

type TestimonialItem = {
  author: string
  quote: string
  role: string
}

type TestimonialsData = {
  items: TestimonialItem[]
}

type VideoEmbedData = {
  title: string
  url: string
}

type SpecsTableData = {
  rows: KeyValueRow[]
}

type StructuredBlockData =
  | ComparisonTableData
  | ImageTextData
  | ProsConsData
  | SpecsTableData
  | TabsBlockData
  | TestimonialsData
  | VideoEmbedData

type SerializedStructuredBlockNode = {
  blockType: StructuredBlockType
  data: StructuredBlockData
  type: 'structured-block'
  version: 1
}

function defaultStructuredBlockData(blockType: StructuredBlockType): StructuredBlockData {
  switch (blockType) {
    case 'pros-cons':
      return { pros: ['First benefit'], cons: ['First limitation'] }
    case 'comparison-table':
      return {
        columns: ['Feature', 'Option A', 'Option B'],
        rows: [
          ['Battery life', '8 hours', '10 hours'],
          ['Weight', '1.2 kg', '1.1 kg'],
        ],
      }
    case 'tabs':
      return {
        tabs: [
          { label: 'Shipping', content: 'Add shipping details here.' },
          { label: 'Returns', content: 'Add return policy details here.' },
        ],
      }
    case 'testimonials':
      return {
        items: [{ quote: 'Add a customer quote here.', author: 'Customer name', role: 'Role or company' }],
      }
    case 'video-embed':
      return { title: 'Product video', url: '' }
    case 'specs-table':
      return {
        rows: [
          { label: 'Material', value: 'Aluminum' },
          { label: 'Weight', value: '1.1 kg' },
        ],
      }
    case 'image-text':
      return {
        title: 'Image section title',
        body: 'Add supporting copy for this image section.',
        imageUrl: '',
        imageAlt: '',
        layout: 'image-left',
      }
  }
}

class StructuredBlockNode extends DecoratorNode<ReactNode> {
  __blockType: StructuredBlockType
  __data: StructuredBlockData

  static getType(): string {
    return 'structured-block'
  }

  static clone(node: StructuredBlockNode): StructuredBlockNode {
    return new StructuredBlockNode(node.__blockType, node.__data, node.__key)
  }

  static importJSON(serializedNode: SerializedStructuredBlockNode): StructuredBlockNode {
    return new StructuredBlockNode(serializedNode.blockType, serializedNode.data)
  }

  constructor(blockType: StructuredBlockType, data: StructuredBlockData, key?: NodeKey) {
    super(key)
    this.__blockType = blockType
    this.__data = data
  }

  exportJSON(): SerializedStructuredBlockNode {
    return {
      blockType: this.__blockType,
      data: this.__data,
      type: 'structured-block',
      version: 1,
    }
  }

  // Block-level - none of the seven structured variants (tabs, pros-cons, comparison-table, etc.) belong inside a
  // paragraph Without these, $insertNodes nests under the cursor's <p>
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

  getBlockType(): StructuredBlockType {
    return this.getLatest().__blockType
  }

  getData(): StructuredBlockData {
    return this.getLatest().__data
  }

  setData(data: StructuredBlockData) {
    const writable = this.getWritable()
    writable.__data = data
  }

  decorate(): ReactNode {
    return <StructuredBlockComponent nodeKey={this.__key} blockType={this.__blockType} data={this.__data} />
  }
}

function $isStructuredBlockNode(node: LexicalNode | null | undefined): node is StructuredBlockNode {
  return node instanceof StructuredBlockNode
}

function StructuredBlockComponent({
  nodeKey,
  blockType,
  data,
}: {
  nodeKey: NodeKey
  blockType: StructuredBlockType
  data: StructuredBlockData
}) {
  const [editor] = useLexicalComposerContext()
  const [editable, setEditable] = useState(() => editor.isEditable())

  useEffect(() => editor.registerEditableListener(setEditable), [editor])

  function updateData(nextData: StructuredBlockData) {
    editor.update(() => {
      const node = $getNodeByKey(nodeKey)
      if ($isStructuredBlockNode(node)) {
        node.setData(nextData)
      }
    })
  }

  return editable
    ? renderStructuredBlockEditor(blockType, data, updateData)
    : renderStructuredBlockPreview(blockType, data)
}

function renderStructuredBlockPreview(blockType: StructuredBlockType, data: StructuredBlockData): ReactNode {
  switch (blockType) {
    case 'pros-cons': {
      const typedData = data as ProsConsData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Pros &amp; Cons</p>
          <div className="grid gap-3 md:grid-cols-2">
            <div className="rounded-md border bg-background p-3">
              <p className="mb-2 text-sm font-medium">Pros</p>
              <ul className="grid gap-2 text-sm text-muted-foreground">
                {typedData.pros.map((item, index) => <li key={index}>• {item}</li>)}
              </ul>
            </div>
            <div className="rounded-md border bg-background p-3">
              <p className="mb-2 text-sm font-medium">Cons</p>
              <ul className="grid gap-2 text-sm text-muted-foreground">
                {typedData.cons.map((item, index) => <li key={index}>• {item}</li>)}
              </ul>
            </div>
          </div>
        </section>
      )
    }
    case 'comparison-table': {
      const typedData = data as ComparisonTableData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Comparison Table</p>
          <Table>
            <TableHeader>
              <TableRow>
                {typedData.columns.map((column, index) => <TableHead key={index}>{column}</TableHead>)}
              </TableRow>
            </TableHeader>
            <TableBody>
              {typedData.rows.map((row, rowIndex) => (
                <TableRow key={rowIndex}>
                  {row.map((cell, cellIndex) => <TableCell key={cellIndex}>{cell}</TableCell>)}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </section>
      )
    }
    case 'tabs': {
      const typedData = data as TabsBlockData
      const firstTab = typedData.tabs[0]?.label || 'tab-1'
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Tabs</p>
          <Tabs defaultValue={firstTab}>
            <TabsList className="mb-3">
              {typedData.tabs.map((tab, index) => (
                <TabsTrigger key={index} value={tab.label || `tab-${index + 1}`}>{tab.label || `Tab ${index + 1}`}</TabsTrigger>
              ))}
            </TabsList>
            {typedData.tabs.map((tab, index) => (
              <TabsContent key={index} value={tab.label || `tab-${index + 1}`} className="rounded-md border bg-background p-3 text-sm text-muted-foreground">
                {tab.content || 'No content yet.'}
              </TabsContent>
            ))}
          </Tabs>
        </section>
      )
    }
    case 'testimonials': {
      const typedData = data as TestimonialsData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Testimonials</p>
          <div className="grid gap-3">
            {typedData.items.map((item, index) => (
              <blockquote key={index} className="rounded-md border bg-background p-4">
                <p className="text-sm">“{item.quote || 'Add quote'}”</p>
                <footer className="mt-3 text-xs text-muted-foreground">
                  {item.author || 'Author'}{item.role ? ` · ${item.role}` : ''}
                </footer>
              </blockquote>
            ))}
          </div>
        </section>
      )
    }
    case 'video-embed': {
      const typedData = data as VideoEmbedData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-2 text-sm font-semibold">{typedData.title || 'Video Embed'}</p>
          <div className="rounded-md border bg-background p-4 text-sm text-muted-foreground">
            {typedData.url ? typedData.url : 'Add a YouTube, Vimeo, or direct video URL.'}
          </div>
        </section>
      )
    }
    case 'specs-table': {
      const typedData = data as SpecsTableData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Specs Table</p>
          <Table>
            <TableBody>
              {typedData.rows.map((row, index) => (
                <TableRow key={index}>
                  <TableHead>{row.label}</TableHead>
                  <TableCell>{row.value}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </section>
      )
    }
    case 'image-text': {
      const typedData = data as ImageTextData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Image + Text</p>
          <div className={cn('grid gap-4 md:grid-cols-2', typedData.layout === 'image-right' && 'md:[&>*:first-child]:order-2')}>
            <div className="rounded-md border bg-background p-4 text-sm text-muted-foreground">
              {typedData.imageUrl ? typedData.imageUrl : 'Add an image URL.'}
            </div>
            <div className="rounded-md border bg-background p-4">
              <p className="text-sm font-medium">{typedData.title || 'Section title'}</p>
              <p className="mt-2 whitespace-pre-wrap text-sm text-muted-foreground">{typedData.body || 'Add supporting copy.'}</p>
            </div>
          </div>
        </section>
      )
    }
  }
}

function renderStructuredBlockEditor(
  blockType: StructuredBlockType,
  data: StructuredBlockData,
  updateData: (nextData: StructuredBlockData) => void,
): ReactNode {
  switch (blockType) {
    case 'pros-cons': {
      const typedData = data as ProsConsData
      const updateList = (key: 'pros' | 'cons', nextValues: string[]) => {
        updateData({ ...typedData, [key]: nextValues })
      }
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">Pros &amp; Cons</p>
              <p className="text-xs text-muted-foreground">Highlight benefits and tradeoffs side by side.</p>
            </div>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            {(['pros', 'cons'] as const).map((key) => (
              <div key={key} className="grid gap-2 rounded-md border bg-background p-3">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{key}</span>
                  <Button type="button" size="sm" variant="outline" onClick={() => updateList(key, [...typedData[key], ''])}>
                    <PlusIcon className="size-4" />
                    Add
                  </Button>
                </div>
                {typedData[key].map((item, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <Input value={item} onChange={(event) => updateList(key, typedData[key].map((value, valueIndex) => valueIndex === index ? event.target.value : value))} placeholder={`${key === 'pros' ? 'Pro' : 'Con'} item`} />
                    <Button aria-label="Remove item" type="button" size="icon" variant="ghost" className="size-8" onClick={() => updateList(key, typedData[key].filter((_, valueIndex) => valueIndex !== index))}>
                      <Trash2Icon className="size-4" />
                    </Button>
                  </div>
                ))}
              </div>
            ))}
          </div>
        </section>
      )
    }
    case 'comparison-table': {
      const typedData = data as ComparisonTableData
      function setColumns(columns: string[]) {
        updateData({
          ...typedData,
          columns,
          rows: typedData.rows.map((row) => {
            const nextRow = row.slice(0, columns.length)
            while (nextRow.length < columns.length) nextRow.push('')
            return nextRow
          }),
        })
      }
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">Comparison Table</p>
              <p className="text-xs text-muted-foreground">Compare options, models, or bundles in a structured matrix.</p>
            </div>
            <div className="flex gap-2">
              <Button type="button" size="sm" variant="outline" onClick={() => setColumns([...typedData.columns, `Column ${typedData.columns.length + 1}`])}>
                <PlusIcon className="size-4" />
                Column
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={() => updateData({ ...typedData, rows: [...typedData.rows, Array.from({ length: typedData.columns.length }, () => '')] })}>
                <PlusIcon className="size-4" />
                Row
              </Button>
            </div>
          </div>
          <div className="grid gap-2 rounded-md border bg-background p-3">
            <div className="grid gap-2" style={{ gridTemplateColumns: `repeat(${typedData.columns.length}, minmax(0, 1fr)) auto` }}>
              {typedData.columns.map((column, index) => (
                <Input key={index} value={column} onChange={(event) => setColumns(typedData.columns.map((value, valueIndex) => valueIndex === index ? event.target.value : value))} placeholder={`Column ${index + 1}`} />
              ))}
              <Button aria-label="Remove last column"
                type="button"
                size="icon"
                variant="ghost"
                className="size-9"
                onClick={() => typedData.columns.length > 1 ? setColumns(typedData.columns.slice(0, -1)) : undefined}
              >
                <Trash2Icon className="size-4" />
              </Button>
            </div>
            {typedData.rows.map((row, rowIndex) => (
              <div key={rowIndex} className="grid gap-2" style={{ gridTemplateColumns: `repeat(${typedData.columns.length}, minmax(0, 1fr)) auto` }}>
                {row.map((cell, cellIndex) => (
                  <Input
                    key={cellIndex}
                    value={cell}
                    onChange={(event) => updateData({
                      ...typedData,
                      rows: typedData.rows.map((existingRow, existingRowIndex) =>
                        existingRowIndex === rowIndex
                          ? existingRow.map((existingCell, existingCellIndex) => existingCellIndex === cellIndex ? event.target.value : existingCell)
                          : existingRow,
                      ),
                    })}
                    placeholder={`Cell ${rowIndex + 1}.${cellIndex + 1}`}
                  />
                ))}
                <Button aria-label="Remove row" type="button" size="icon" variant="ghost" className="size-9" onClick={() => updateData({ ...typedData, rows: typedData.rows.filter((_, existingRowIndex) => existingRowIndex !== rowIndex) })}>
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            ))}
          </div>
        </section>
      )
    }
    case 'tabs': {
      const typedData = data as TabsBlockData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">Tabs</p>
              <p className="text-xs text-muted-foreground">Group related product details into labeled sections.</p>
            </div>
            <Button type="button" size="sm" variant="outline" onClick={() => updateData({ ...typedData, tabs: [...typedData.tabs, { label: '', content: '' }] })}>
              <PlusIcon className="size-4" />
              Add tab
            </Button>
          </div>
          <div className="grid gap-3">
            {typedData.tabs.map((tab, index) => (
              <div key={index} className="grid gap-2 rounded-md border bg-background p-3">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Tab {index + 1}</span>
                  <Button aria-label={`Remove tab ${index + 1}`} type="button" size="icon" variant="ghost" className="size-8" onClick={() => updateData({ ...typedData, tabs: typedData.tabs.filter((_, tabIndex) => tabIndex !== index) })}>
                    <Trash2Icon className="size-4" />
                  </Button>
                </div>
                <Input value={tab.label} onChange={(event) => updateData({ ...typedData, tabs: typedData.tabs.map((item, itemIndex) => itemIndex === index ? { ...item, label: event.target.value } : item) })} placeholder="Tab label" />
                <Textarea value={tab.content} onChange={(event) => updateData({ ...typedData, tabs: typedData.tabs.map((item, itemIndex) => itemIndex === index ? { ...item, content: event.target.value } : item) })} placeholder="Tab content" rows={4} />
              </div>
            ))}
          </div>
        </section>
      )
    }
    case 'testimonials': {
      const typedData = data as TestimonialsData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">Testimonials</p>
              <p className="text-xs text-muted-foreground">Collect customer quotes, names, and roles.</p>
            </div>
            <Button type="button" size="sm" variant="outline" onClick={() => updateData({ ...typedData, items: [...typedData.items, { quote: '', author: '', role: '' }] })}>
              <PlusIcon className="size-4" />
              Add testimonial
            </Button>
          </div>
          <div className="grid gap-3">
            {typedData.items.map((item, index) => (
              <div key={index} className="grid gap-2 rounded-md border bg-background p-3">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Testimonial {index + 1}</span>
                  <Button aria-label={`Remove testimonial ${index + 1}`} type="button" size="icon" variant="ghost" className="size-8" onClick={() => updateData({ ...typedData, items: typedData.items.filter((_, itemIndex) => itemIndex !== index) })}>
                    <Trash2Icon className="size-4" />
                  </Button>
                </div>
                <Textarea value={item.quote} onChange={(event) => updateData({ ...typedData, items: typedData.items.map((existing, existingIndex) => existingIndex === index ? { ...existing, quote: event.target.value } : existing) })} placeholder="Quote" rows={4} />
                <div className="grid gap-2 md:grid-cols-2">
                  <Input value={item.author} onChange={(event) => updateData({ ...typedData, items: typedData.items.map((existing, existingIndex) => existingIndex === index ? { ...existing, author: event.target.value } : existing) })} placeholder="Author" />
                  <Input value={item.role} onChange={(event) => updateData({ ...typedData, items: typedData.items.map((existing, existingIndex) => existingIndex === index ? { ...existing, role: event.target.value } : existing) })} placeholder="Role or company" />
                </div>
              </div>
            ))}
          </div>
        </section>
      )
    }
    case 'video-embed': {
      const typedData = data as VideoEmbedData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Video Embed</p>
          <div className="grid gap-2 rounded-md border bg-background p-3">
            <Input value={typedData.title} onChange={(event) => updateData({ ...typedData, title: event.target.value })} placeholder="Block title" />
            <Input value={typedData.url} onChange={(event) => updateData({ ...typedData, url: event.target.value })} placeholder="Video URL" />
            <p className="text-xs text-muted-foreground">Use a public YouTube, Vimeo, or direct video URL. This block currently stores the URL as JSON.</p>
          </div>
        </section>
      )
    }
    case 'specs-table': {
      const typedData = data as SpecsTableData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <p className="text-sm font-semibold">Specs Table</p>
              <p className="text-xs text-muted-foreground">Store key product specifications as labeled rows.</p>
            </div>
            <Button type="button" size="sm" variant="outline" onClick={() => updateData({ ...typedData, rows: [...typedData.rows, { label: '', value: '' }] })}>
              <PlusIcon className="size-4" />
              Add row
            </Button>
          </div>
          <div className="grid gap-2 rounded-md border bg-background p-3">
            {typedData.rows.map((row, index) => (
              <div key={index} className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
                <Input value={row.label} onChange={(event) => updateData({ ...typedData, rows: typedData.rows.map((existing, existingIndex) => existingIndex === index ? { ...existing, label: event.target.value } : existing) })} placeholder="Label" />
                <Input value={row.value} onChange={(event) => updateData({ ...typedData, rows: typedData.rows.map((existing, existingIndex) => existingIndex === index ? { ...existing, value: event.target.value } : existing) })} placeholder="Value" />
                <Button aria-label="Remove row" type="button" size="icon" variant="ghost" className="size-9" onClick={() => updateData({ ...typedData, rows: typedData.rows.filter((_, existingIndex) => existingIndex !== index) })}>
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            ))}
          </div>
        </section>
      )
    }
    case 'image-text': {
      const typedData = data as ImageTextData
      return (
        <section className="my-4 rounded-lg border bg-muted/20 p-4">
          <p className="mb-3 text-sm font-semibold">Image + Text</p>
          <div className="grid gap-2 rounded-md border bg-background p-3">
            <div className="grid gap-2 md:grid-cols-2">
              <Input value={typedData.imageUrl} onChange={(event) => updateData({ ...typedData, imageUrl: event.target.value })} placeholder="Image URL" />
              <Input value={typedData.imageAlt} onChange={(event) => updateData({ ...typedData, imageAlt: event.target.value })} placeholder="Image alt text" />
            </div>
            <Input value={typedData.title} onChange={(event) => updateData({ ...typedData, title: event.target.value })} placeholder="Section title" />
            <Textarea value={typedData.body} onChange={(event) => updateData({ ...typedData, body: event.target.value })} placeholder="Body text" rows={5} />
            <div className="flex gap-2">
              <Button type="button" size="sm" variant={typedData.layout === 'image-left' ? 'default' : 'outline'} onClick={() => updateData({ ...typedData, layout: 'image-left' })}>
                Image left
              </Button>
              <Button type="button" size="sm" variant={typedData.layout === 'image-right' ? 'default' : 'outline'} onClick={() => updateData({ ...typedData, layout: 'image-right' })}>
                Image right
              </Button>
            </div>
          </div>
        </section>
      )
    }
  }
}

function insertStructuredBlock(editor: LexicalEditor, blockType: StructuredBlockType) {
  editor.update(() => {
    const selection = $getSelection()
    const blockNode = new StructuredBlockNode(blockType, defaultStructuredBlockData(blockType))
    const trailingParagraph = $createParagraphNode()

    if ($isRangeSelection(selection)) {
      $insertNodes([blockNode, trailingParagraph])
      return
    }
    $getRoot().append(blockNode, trailingParagraph)
  })
}

// Per-variant labels for the dropdown - The shared node class only
// needs to register once; the registry dedupes by class identity
const VARIANTS: { type: StructuredBlockType; label: string }[] = [
  { type: 'pros-cons', label: 'Pros & Cons' },
  { type: 'comparison-table', label: 'Comparison Table' },
  { type: 'tabs', label: 'Tabs' },
  { type: 'testimonials', label: 'Testimonials' },
  { type: 'video-embed', label: 'Video Embed' },
  { type: 'specs-table', label: 'Specs Table' },
  { type: 'image-text', label: 'Image + Text' },
]

// First variant carries the node class; subsequent ones omit it The registry's Set semantics make multi-register safe,
// but skipping the redundant work keeps the intent clear
registerLexicalBlock({
  node: StructuredBlockNode,
  toolbar: {
    id: `core:structured-${VARIANTS[0].type}`,
    label: VARIANTS[0].label,
    section: 'Structured',
    insert: (editor) => insertStructuredBlock(editor, VARIANTS[0].type),
  },
  extractPlainText: {
    nodeType: 'structured-block',
    extract: (node) => {
      const blockType = (node as { blockType?: unknown }).blockType
      const data = (node as { data?: unknown }).data
      if (!data || typeof data !== 'object') return ''
      switch (blockType) {
        case 'pros-cons': {
          const d = data as Partial<ProsConsData>
          return [...(d.pros ?? []), ...(d.cons ?? [])].join(' ')
        }
        case 'comparison-table': {
          const d = data as Partial<ComparisonTableData>
          return [...(d.columns ?? []), ...((d.rows ?? []).flat())].join(' ')
        }
        case 'tabs': {
          const d = data as Partial<TabsBlockData>
          return (d.tabs ?? []).map((t) => [t.label, t.content].filter(Boolean).join(' ')).join(' ')
        }
        case 'testimonials': {
          const d = data as Partial<TestimonialsData>
          return (d.items ?? []).map((i) => [i.quote, i.author, i.role].filter(Boolean).join(' ')).join(' ')
        }
        case 'video-embed': {
          const d = data as Partial<VideoEmbedData>
          return [d.title, d.url].filter(Boolean).join(' ')
        }
        case 'specs-table': {
          const d = data as Partial<SpecsTableData>
          return (d.rows ?? []).map((r) => [r.label, r.value].filter(Boolean).join(' ')).join(' ')
        }
        case 'image-text': {
          const d = data as Partial<ImageTextData>
          return [d.title, d.body, d.imageAlt, d.imageUrl].filter(Boolean).join(' ')
        }
        default:
          return ''
      }
    },
  },
})

for (const variant of VARIANTS.slice(1)) {
  registerLexicalBlock({
    toolbar: {
      id: `core:structured-${variant.type}`,
      label: variant.label,
      section: 'Structured',
      insert: (editor) => insertStructuredBlock(editor, variant.type),
    },
  })
}
