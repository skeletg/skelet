/* eslint-disable react-refresh/only-export-components */
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ReactNode } from 'react'
import { $isLinkNode, LinkNode, TOGGLE_LINK_COMMAND } from '@lexical/link'
import { ListItemNode, ListNode, $isListNode, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND } from '@lexical/list'
import { HEADING, LINK, ORDERED_LIST, QUOTE, TEXT_FORMAT_TRANSFORMERS, UNORDERED_LIST } from '@lexical/markdown'
import { MarkdownShortcutPlugin } from '@lexical/react/LexicalMarkdownShortcutPlugin'
import { LexicalComposer, type InitialConfigType } from '@lexical/react/LexicalComposer'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import { ContentEditable } from '@lexical/react/LexicalContentEditable'
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary'
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin'
import { LinkPlugin } from '@lexical/react/LexicalLinkPlugin'
import { ListPlugin } from '@lexical/react/LexicalListPlugin'
import { OnChangePlugin } from '@lexical/react/LexicalOnChangePlugin'
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin'
import { $createHeadingNode, $createQuoteNode, $isHeadingNode, $isQuoteNode, HeadingNode, QuoteNode, type HeadingTagType } from '@lexical/rich-text'
import { $setBlocksType } from '@lexical/selection'
import { $findMatchingParent } from '@lexical/utils'
import {
  $createParagraphNode,
  $getRoot,
  $getSelection,
  $insertNodes,
  $isElementNode,
  $isRangeSelection,
  COMMAND_PRIORITY_LOW,
  FORMAT_TEXT_COMMAND,
  SELECTION_CHANGE_COMMAND,
  type LexicalEditor,
  type SerializedEditorState,
} from 'lexical'
import {
  BoldIcon,
  ChevronDownIcon,
  ImageIcon,
  ItalicIcon,
  LinkIcon,
  ListIcon,
  ListOrderedIcon,
  PilcrowIcon,
  QuoteIcon,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

// Importing the lexical-blocks barrel fires the core blocks' top-level registerLexicalBlock calls - Plugin-registered
// blocks arrive via src/plugins/loader.ts (imported from App.tsx)
import {
  $createProductImageNode,
  extractPlainTextFor,
  getLexicalBlockNodes,
  getLexicalBlockToolbarEntries,
  type LexicalBlockToolbarEntry,
} from './lexical-blocks'

export type RichTextValue = SerializedEditorState | string | null
export type RichTextImageUploadResult = {
  altText?: string
  height: number
  src: string
  width: number
}

const editorTheme = {
  heading: {
    h1: 'mb-3 text-xl font-semibold',
    h2: 'mb-2 text-lg font-semibold',
    h3: 'mb-2 text-base font-semibold',
    h4: 'mb-2 text-sm font-semibold',
    h5: 'mb-2 text-sm font-medium',
    h6: 'mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground',
  },
  link: 'text-primary underline underline-offset-2',
  list: {
    listitem: 'ml-5',
    nested: {
      listitem: 'list-none',
    },
    ol: 'my-2 list-decimal pl-5',
    ul: 'my-2 list-disc pl-5',
  },
  paragraph: 'mb-2 last:mb-0',
  quote: 'my-2 border-l-2 pl-3 text-muted-foreground',
  text: {
    bold: 'font-semibold',
    italic: 'italic',
    underline: 'underline',
  },
}

const HEADING_OPTIONS: HeadingTagType[] = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6']

const markdownTransformers = [
  HEADING,
  QUOTE,
  UNORDERED_LIST,
  ORDERED_LIST,
  ...TEXT_FORMAT_TRANSFORMERS,
  LINK,
]

function textNode(text: string) {
  return { detail: 0, format: 0, mode: 'normal', style: '', text, type: 'text', version: 1 }
}

function paragraphNode(text = '') {
  return {
    children: text ? [textNode(text)] : [],
    direction: null,
    format: '',
    indent: 0,
    type: 'paragraph',
    version: 1,
  }
}

function plainTextToEditorStateValue(text: string): SerializedEditorState {
  const lines = text ? text.split(/\r?\n/) : ['']
  return {
    root: {
      children: lines.map((line) => paragraphNode(line)),
      direction: null,
      format: '',
      indent: 0,
      type: 'root',
      version: 1,
    },
  } as SerializedEditorState
}

function isSerializedEditorState(value: RichTextValue | undefined): value is SerializedEditorState {
  return Boolean(value && typeof value === 'object' && 'root' in value)
}

function plainTextToEditorState(text: string): string {
  return JSON.stringify(plainTextToEditorStateValue(text))
}

// textFromSerializedNode walks the saved Lexical tree extracting
// plain text - Built-in branches: string leaves + recursive descend
// into `children` - Anything with a `type` that a custom block has
// claimed via registerLexicalBlock({ extractPlainText: ... }) goes
// through that block's extractor instead
function textFromSerializedNode(node: unknown): string {
  if (!node || typeof node !== 'object') return ''

  const maybeText = (node as { text?: unknown }).text
  if (typeof maybeText === 'string') return maybeText

  const maybeType = (node as { type?: unknown }).type
  if (typeof maybeType === 'string') {
    const extractor = extractPlainTextFor(maybeType)
    if (extractor) return extractor(node)
  }

  const maybeChildren = (node as { children?: unknown }).children
  if (!Array.isArray(maybeChildren)) return ''
  return maybeChildren.map(textFromSerializedNode).filter(Boolean).join(' ')
}

export function richTextToPlainText(value: RichTextValue | undefined): string {
  if (!value) return ''
  if (typeof value === 'string') return value
  return textFromSerializedNode(value.root).replace(/\s+/g, ' ').trim()
}

function toInitialEditorState(value: RichTextValue | undefined, fallbackText: string): string {
  if (isSerializedEditorState(value)) return JSON.stringify(value)
  if (typeof value === 'string') return plainTextToEditorState(value)
  return plainTextToEditorState(fallbackText)
}

function EditablePlugin({ editable }: { editable: boolean }) {
  const [editor] = useLexicalComposerContext()

  useEffect(() => {
    editor.setEditable(editable)
  }, [editable, editor])

  return null
}

function ToolbarButton({
  active,
  label,
  disabled,
  onClick,
  children,
}: {
  active?: boolean
  label: string
  disabled?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={cn('size-8', active && 'bg-accent text-accent-foreground ring-1 ring-border')}
      disabled={disabled}
      onClick={onClick}
      aria-label={label}
      title={label}
      aria-pressed={active}
    >
      {children}
    </Button>
  )
}

function setBlockType(editor: LexicalEditor, type: 'paragraph' | 'quote' | HeadingTagType) {
  editor.update(() => {
    const selection = $getSelection()
    if (!$isRangeSelection(selection)) return

    if (type === 'paragraph') {
      $setBlocksType(selection, () => $createParagraphNode())
    } else if (type === 'quote') {
      $setBlocksType(selection, () => $createQuoteNode())
    } else {
      $setBlocksType(selection, () => $createHeadingNode(type))
    }
  })
}

function insertOrEditLink(editor: LexicalEditor) {
  const url = window.prompt('Link URL')
  if (url == null) return
  editor.dispatchCommand(TOGGLE_LINK_COMMAND, url.trim() || null)
}

function blockTypeLabel(blockType: ToolbarState['blockType']) {
  if (blockType === 'paragraph') return 'Paragraph'
  if (blockType === 'quote') return 'Quote'
  if (blockType === 'other') return 'Format'
  return blockType.toUpperCase()
}

type ToolbarState = {
  blockType: 'paragraph' | 'quote' | HeadingTagType | 'other'
  isBold: boolean
  isBulletList: boolean
  isItalic: boolean
  isLink: boolean
  isNumberedList: boolean
}

function getToolbarState(editor: LexicalEditor): ToolbarState {
  return editor.getEditorState().read(() => {
    const selection = $getSelection()
    if (!$isRangeSelection(selection)) {
      return {
        blockType: 'other',
        isBold: false,
        isBulletList: false,
        isItalic: false,
        isLink: false,
        isNumberedList: false,
      }
    }

    const anchorNode = selection.anchor.getNode()
    const selectedNode = $isElementNode(anchorNode) ? anchorNode : anchorNode.getParent()
    const topLevelElement = selectedNode?.getTopLevelElementOrThrow() ?? null
    const listNode = selectedNode
      ? $findMatchingParent(selectedNode, (node) => $isListNode(node))
      : null
    const linkNode = $findMatchingParent(anchorNode, (node) => $isLinkNode(node))

    let blockType: ToolbarState['blockType'] = 'other'
    if (topLevelElement) {
      if ($isHeadingNode(topLevelElement)) {
        blockType = topLevelElement.getTag()
      } else if ($isQuoteNode(topLevelElement)) {
        blockType = 'quote'
      } else if (topLevelElement.getType() === 'paragraph') {
        blockType = 'paragraph'
      }
    }

    return {
      blockType,
      isBold: selection.hasFormat('bold'),
      isBulletList: $isListNode(listNode) && listNode.getListType() === 'bullet',
      isItalic: selection.hasFormat('italic'),
      isLink: Boolean(linkNode),
      isNumberedList: $isListNode(listNode) && listNode.getListType() === 'number',
    }
  })
}

function insertImage(
  editor: LexicalEditor,
  image: RichTextImageUploadResult,
) {
  editor.update(() => {
    const selection = $getSelection()
    const imageNode = $createProductImageNode(
      image.src,
      image.altText?.trim() || '',
      image.width,
      image.height,
    )
    const trailingParagraph = $createParagraphNode()

    if ($isRangeSelection(selection)) {
      $insertNodes([imageNode, trailingParagraph])
      return
    }

    $getRoot().append(imageNode, trailingParagraph)
  })
}

// groupedToolbarEntries bucket toolbar entries by their `section` field in first-seen order - Entries with no section land in a trailing
// unnamed bucket, rendered without a heading - Keeps the dropdown render loop simple and stable across registrations
function groupedToolbarEntries(entries: LexicalBlockToolbarEntry[]): {
  section: string
  entries: LexicalBlockToolbarEntry[]
}[] {
  const order: string[] = []
  const map = new Map<string, LexicalBlockToolbarEntry[]>()
  for (const entry of entries) {
    const section = entry.section ?? ''
    if (!map.has(section)) {
      order.push(section)
      map.set(section, [])
    }
    map.get(section)!.push(entry)
  }
  return order.map((section) => ({ section, entries: map.get(section)! }))
}

function EditorToolbar({
  disabled,
  onUploadImage,
  onUploadImageError,
  blockEntries,
}: {
  disabled?: boolean
  onUploadImage?: (file: File) => Promise<RichTextImageUploadResult | null>
  onUploadImageError: (message: string | null) => void
  blockEntries: LexicalBlockToolbarEntry[]
}) {
  const [editor] = useLexicalComposerContext()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [uploadingImage, setUploadingImage] = useState(false)
  const [toolbarState, setToolbarState] = useState<ToolbarState>(() => getToolbarState(editor))

  useEffect(() => {
    const unregisterSelection = editor.registerCommand(
      SELECTION_CHANGE_COMMAND,
      () => {
        setToolbarState(getToolbarState(editor))
        return false
      },
      COMMAND_PRIORITY_LOW,
    )
    const unregisterUpdate = editor.registerUpdateListener(() => {
      setToolbarState(getToolbarState(editor))
    })

    return () => {
      unregisterSelection()
      unregisterUpdate()
    }
  }, [editor])

  async function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file || !onUploadImage) return

    setUploadingImage(true)
    onUploadImageError(null)
    try {
      const uploaded = await onUploadImage(file)
      if (uploaded) insertImage(editor, uploaded)
    } catch (error) {
      onUploadImageError(error instanceof Error ? error.message : 'Image upload failed.')
    } finally {
      setUploadingImage(false)
    }
  }

  const grouped = useMemo(() => groupedToolbarEntries(blockEntries), [blockEntries])

  return (
    <div className="flex flex-wrap items-center gap-1 border-b bg-muted/30 p-1.5">
      <ToolbarButton
        label="Bold"
        active={toolbarState.isBold}
        disabled={disabled}
        onClick={() => editor.dispatchCommand(FORMAT_TEXT_COMMAND, 'bold')}
      >
        <BoldIcon className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label="Italic"
        active={toolbarState.isItalic}
        disabled={disabled}
        onClick={() => editor.dispatchCommand(FORMAT_TEXT_COMMAND, 'italic')}
      >
        <ItalicIcon className="size-4" />
      </ToolbarButton>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className={cn(
              'h-8 gap-2 px-2 text-xs font-semibold',
              toolbarState.blockType !== 'other' && 'bg-accent text-accent-foreground ring-1 ring-border',
            )}
            disabled={disabled}
          >
            {toolbarState.blockType === 'quote' ? <QuoteIcon className="size-4" /> : <PilcrowIcon className="size-4" />}
            <span>{blockTypeLabel(toolbarState.blockType)}</span>
            <ChevronDownIcon className="size-3" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem onClick={() => setBlockType(editor, 'paragraph')}>
            Paragraph
          </DropdownMenuItem>
          {HEADING_OPTIONS.map((heading) => (
            <DropdownMenuItem key={heading} onClick={() => setBlockType(editor, heading)}>
              {heading.toUpperCase()}
            </DropdownMenuItem>
          ))}
          <DropdownMenuItem onClick={() => setBlockType(editor, 'quote')}>
            Quote
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ToolbarButton
        label="Bulleted list"
        active={toolbarState.isBulletList}
        disabled={disabled}
        onClick={() => editor.dispatchCommand(INSERT_UNORDERED_LIST_COMMAND, undefined)}
      >
        <ListIcon className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label="Numbered list"
        active={toolbarState.isNumberedList}
        disabled={disabled}
        onClick={() => editor.dispatchCommand(INSERT_ORDERED_LIST_COMMAND, undefined)}
      >
        <ListOrderedIcon className="size-4" />
      </ToolbarButton>
      <ToolbarButton
        label="Link"
        active={toolbarState.isLink}
        disabled={disabled}
        onClick={() => insertOrEditLink(editor)}
      >
        <LinkIcon className="size-4" />
      </ToolbarButton>
      {onUploadImage ? (
        <>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={onFileChange}
          />
          <ToolbarButton
            label={uploadingImage ? 'Uploading image...' : 'Add image'}
            disabled={disabled || uploadingImage}
            onClick={() => fileInputRef.current?.click()}
          >
            <ImageIcon className="size-4" />
          </ToolbarButton>
        </>
      ) : null}
      {blockEntries.length > 0 ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="sm" className="h-8 gap-2 px-2 text-xs font-semibold" disabled={disabled}>
              <span>Custom Blocks</span>
              <ChevronDownIcon className="size-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {grouped.map((group, groupIndex) => (
              <div key={group.section || `section-${groupIndex}`}>
                {groupIndex > 0 ? <DropdownMenuSeparator /> : null}
                {group.section ? (
                  <DropdownMenuLabel className="text-[10px] uppercase tracking-wider text-muted-foreground">
                    {group.section}
                  </DropdownMenuLabel>
                ) : null}
                {group.entries.map((entry) => {
                  const Icon = entry.icon
                  return (
                    <DropdownMenuItem key={entry.id} onClick={() => entry.insert(editor)}>
                      {Icon ? <Icon className="size-4" /> : null}
                      {entry.label}
                    </DropdownMenuItem>
                  )
                })}
              </div>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  )
}

export function LexicalRichTextEditor({
  value,
  fallbackText,
  disabled,
  placeholder = 'Write product description...',
  contentClassName,
  onUploadImage,
  onChange,
}: {
  value?: RichTextValue
  fallbackText: string
  disabled?: boolean
  placeholder?: string
  contentClassName?: string
  onUploadImage?: (file: File) => Promise<RichTextImageUploadResult | null>
  onChange: (value: RichTextValue, plainText: string) => void
}) {
  const [uploadError, setUploadError] = useState<string | null>(null)
  // Snapshot the registry once per editor instance - Late-registering
  // plugins after the editor has mounted is unsupported (the
  // LexicalComposer nodes array is fixed at construction time);
  // every plugin registers at module init via src/plugins/loader.ts,
  // long before any editor mounts
  const blockEntries = useMemo(() => getLexicalBlockToolbarEntries(), [])
  const [initialConfig] = useState<InitialConfigType>(() => ({
    editable: !disabled,
    editorState: toInitialEditorState(value, fallbackText),
    namespace: 'ProductDescriptionEditor',
    // Block nodes from the plugin registry are real Lexical nodes at runtime but typed as bare constructors (no static
    // Klass surface), so assert the assembled list to the composer's expected node type
    nodes: [
      HeadingNode,
      QuoteNode,
      ListNode,
      ListItemNode,
      LinkNode,
      ...getLexicalBlockNodes(),
    ] as unknown as InitialConfigType['nodes'],
    onError(error: Error) {
      throw error
    },
    theme: editorTheme,
  }))

  return (
    <LexicalComposer initialConfig={initialConfig}>
      <div className={cn('overflow-hidden rounded-md border bg-card', disabled && 'opacity-80')}>
        <EditorToolbar
          disabled={disabled}
          onUploadImage={onUploadImage}
          onUploadImageError={setUploadError}
          blockEntries={blockEntries}
        />
        {uploadError ? (
          <p className="border-b bg-destructive/5 px-3 py-2 text-xs text-destructive">{uploadError}</p>
        ) : null}
        <div className="relative flex max-h-[80vh] min-h-48 resize-y flex-col overflow-auto">
          <RichTextPlugin
            contentEditable={
              <ContentEditable
                className={cn('min-h-48 flex-1 px-3 py-3 text-sm leading-6 outline-none', contentClassName)}
                aria-placeholder={placeholder}
                placeholder={
                  <p className="pointer-events-none absolute left-3 top-3 text-sm text-muted-foreground">
                    {placeholder}
                  </p>
                }
              />
            }
            ErrorBoundary={LexicalErrorBoundary}
          />
        </div>
        <HistoryPlugin />
        <ListPlugin />
        <LinkPlugin />
        <MarkdownShortcutPlugin transformers={markdownTransformers} />
        <EditablePlugin editable={!disabled} />
        <OnChangePlugin
          ignoreSelectionChange
          onChange={(editorState) => {
            editorState.read(() => {
              onChange(editorState.toJSON(), $getRoot().getTextContent())
            })
          }}
        />
      </div>
    </LexicalComposer>
  )
}

// The editor for a rich text held in a form. LexicalRichTextEditor reads `value` only when it mounts,
// so a value put there from outside - a restored draft, a discard - would sit in the form while the
// editor kept showing the old copy. This remounts it for those, and never for its own edits
export function FormRichTextEditor(props: Parameters<typeof LexicalRichTextEditor>[0]) {
  const { value, onChange } = props
  const [emitted, setEmitted] = useState(value)
  const [seen, setSeen] = useState(value)
  const [revision, setRevision] = useState(0)
  if (value !== seen) {
    setSeen(value)
    if (value !== emitted) setRevision((r) => r + 1)
  }
  return (
    <LexicalRichTextEditor
      key={revision}
      {...props}
      onChange={(next, plainText) => {
        setEmitted(next)
        onChange(next, plainText)
      }}
    />
  )
}
