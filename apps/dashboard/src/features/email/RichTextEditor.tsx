import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { $generateHtmlFromNodes, $generateNodesFromDOM } from '@lexical/html'
import { $isLinkNode, LinkNode, TOGGLE_LINK_COMMAND } from '@lexical/link'
import { $isListNode, INSERT_ORDERED_LIST_COMMAND, INSERT_UNORDERED_LIST_COMMAND, ListItemNode, ListNode, REMOVE_LIST_COMMAND } from '@lexical/list'
import { LexicalComposer, type InitialConfigType } from '@lexical/react/LexicalComposer'
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext'
import { ContentEditable } from '@lexical/react/LexicalContentEditable'
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary'
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin'
import { LinkPlugin } from '@lexical/react/LexicalLinkPlugin'
import { ListPlugin } from '@lexical/react/LexicalListPlugin'
import { OnChangePlugin } from '@lexical/react/LexicalOnChangePlugin'
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin'
import { $findMatchingParent, $getNearestNodeOfType } from '@lexical/utils'
import {
  $createParagraphNode,
  $getRoot,
  $getSelection,
  $isElementNode,
  $isRangeSelection,
  FORMAT_TEXT_COMMAND,
  TextNode,
  type LexicalEditor,
  type LexicalNode,
  type TextFormatType,
} from 'lexical'
import { BoldIcon, ItalicIcon, LinkIcon, ListIcon, ListOrderedIcon, StrikethroughIcon, UnderlineIcon, UnlinkIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { MergeTagMenu } from './MergeTagMenu'
import { $createMergeTagNode, MERGE_TAG_FORMATS, MergeTagNode } from './MergeTagNode'
import { normalizeRichHtml } from './richtext'

const MERGE_TAG = /\{\{\s*([\w.]+)\s*\}\}/

function MergeTagTransformPlugin() {
  const [editor] = useLexicalComposerContext()
  useEffect(
    () =>
      editor.registerNodeTransform(TextNode, (node) => {
        const m = MERGE_TAG.exec(node.getTextContent())
        if (!m || m.index === undefined) return
        const start = m.index
        const parts = node.splitText(start, start + m[0].length)
        const target = start === 0 ? parts[0] : parts[1]
        if (target) target.replace($createMergeTagNode(m[1], MERGE_TAG_FORMATS.filter((format) => target.hasFormat(format))))
      }),
    [editor],
  )
  return null
}

function OnSave({ onChange, initial }: { onChange: (html: string) => void; initial: string }) {
  const [editor] = useLexicalComposerContext()
  const last = useRef(initial)
  return (
    <OnChangePlugin
      ignoreSelectionChange
      onChange={(state) => {
        state.read(() => {
          const html = normalizeRichHtml($generateHtmlFromNodes(editor, null))
          // Selecting a block mounts the editor - only a real edit counts as one
          if (html !== last.current) {
            last.current = html
            onChange(html)
          }
        })
      }}
    />
  )
}

type Formats = { bold: boolean; italic: boolean; underline: boolean; strikethrough: boolean; link: string | null; list: 'bullet' | 'number' | null }
const NO_FORMATS: Formats = { bold: false, italic: false, underline: false, strikethrough: false, link: null, list: null }

function readFormats(editor: LexicalEditor): Formats {
  return editor.getEditorState().read(() => {
    const sel = $getSelection()
    if (!$isRangeSelection(sel)) return NO_FORMATS
    const anchor = sel.anchor.getNode()
    const link = $findMatchingParent(anchor, $isLinkNode)
    const list = $getNearestNodeOfType(anchor, ListNode)
    return {
      bold: sel.hasFormat('bold'),
      italic: sel.hasFormat('italic'),
      underline: sel.hasFormat('underline'),
      strikethrough: sel.hasFormat('strikethrough'),
      link: link && $isLinkNode(link) ? link.getURL() : null,
      list: list && $isListNode(list) ? (list.getListType() === 'number' ? 'number' : 'bullet') : null,
    }
  })
}

function ToolButton({ label, active, onClick, children }: { label: string; active?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={label}
      aria-pressed={active}
      title={label}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
      className={cn('size-7', active && 'bg-accent text-accent-foreground')}
    >
      {children}
    </Button>
  )
}

function LinkButton({ editor, current }: { editor: LexicalEditor; current: string | null }) {
  const [open, setOpen] = useState(false)
  const [url, setUrl] = useState('')
  function apply(next: string | null) {
    editor.dispatchCommand(TOGGLE_LINK_COMMAND, next)
    setOpen(false)
  }
  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (o) setUrl(current ?? 'https://')
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={current ? 'Edit link' : 'Add link'}
          title={current ? 'Edit link' : 'Add link - select some text first'}
          onMouseDown={(e) => e.preventDefault()}
          className={cn('size-7', current && 'bg-accent text-accent-foreground')}
        >
          <LinkIcon className="size-3.5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="grid w-80 gap-2 p-3"
        onOpenAutoFocus={(e) => e.preventDefault()}
        onCloseAutoFocus={(e) => {
          e.preventDefault()
          editor.focus()
        }}
      >
        <form
          className="flex items-center gap-1.5"
          onSubmit={(e) => {
            e.preventDefault()
            apply(url.trim() || null)
          }}
        >
          <Input aria-label="Link address" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://… or {{storefront.url}}" className="h-8" />
          <Button type="submit" size="sm" className="h-8">
            Apply
          </Button>
        </form>
        {current ? (
          <Button type="button" variant="ghost" size="sm" className="h-7 justify-self-start text-xs" onClick={() => apply(null)}>
            <UnlinkIcon className="size-3.5" /> Remove link
          </Button>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}

function Toolbar({ tags }: { tags: string[] }) {
  const [editor] = useLexicalComposerContext()
  const [formats, setFormats] = useState<Formats>(NO_FORMATS)
  useEffect(() => editor.registerUpdateListener(() => setFormats(readFormats(editor))), [editor])
  const format = (f: TextFormatType) => editor.dispatchCommand(FORMAT_TEXT_COMMAND, f)
  const list = (kind: 'bullet' | 'number') =>
    editor.dispatchCommand(formats.list === kind ? REMOVE_LIST_COMMAND : kind === 'bullet' ? INSERT_UNORDERED_LIST_COMMAND : INSERT_ORDERED_LIST_COMMAND, undefined)
  function insertTag(tag: string) {
    editor.focus()
    editor.update(() => {
      const sel = $getSelection()
      if ($isRangeSelection(sel)) sel.insertNodes([$createMergeTagNode(tag)])
    })
  }
  return (
    <div
      role="toolbar"
      aria-label="Text formatting"
      className="absolute left-0 top-full z-30 mt-2 flex items-center gap-0.5 rounded-md border bg-background p-0.5 font-sans text-foreground shadow-md"
      onClick={(e) => e.stopPropagation()}
    >
      <ToolButton label="Bold" active={formats.bold} onClick={() => format('bold')}>
        <BoldIcon className="size-3.5" />
      </ToolButton>
      <ToolButton label="Italic" active={formats.italic} onClick={() => format('italic')}>
        <ItalicIcon className="size-3.5" />
      </ToolButton>
      <ToolButton label="Underline" active={formats.underline} onClick={() => format('underline')}>
        <UnderlineIcon className="size-3.5" />
      </ToolButton>
      <ToolButton label="Strikethrough" active={formats.strikethrough} onClick={() => format('strikethrough')}>
        <StrikethroughIcon className="size-3.5" />
      </ToolButton>
      <span className="mx-0.5 h-4 w-px bg-border" aria-hidden />
      <LinkButton editor={editor} current={formats.link} />
      <ToolButton label="Bulleted list" active={formats.list === 'bullet'} onClick={() => list('bullet')}>
        <ListIcon className="size-3.5" />
      </ToolButton>
      <ToolButton label="Numbered list" active={formats.list === 'number'} onClick={() => list('number')}>
        <ListOrderedIcon className="size-3.5" />
      </ToolButton>
      <span className="mx-0.5 h-4 w-px bg-border" aria-hidden />
      <MergeTagMenu tags={tags} onPick={insertTag} />
    </div>
  )
}

const theme = {
  paragraph: 'mb-3 last:mb-0',
  text: { bold: 'font-bold', italic: 'italic', underline: 'underline', strikethrough: 'line-through', underlineStrikethrough: 'underline line-through' },
  link: 'underline [color:var(--email-link,inherit)]',
  list: { ul: 'mb-3 list-disc pl-5', ol: 'mb-3 list-decimal pl-5', listitem: 'my-0.5' },
}

function loadHtml(editor: LexicalEditor, html: string) {
  const dom = new DOMParser().parseFromString(`<!doctype html><body>${html}</body>`, 'text/html')
  const root = $getRoot()
  root.clear()
  let paragraph: ReturnType<typeof $createParagraphNode> | null = null
  for (const node of $generateNodesFromDOM(editor, dom) as LexicalNode[]) {
    if ($isElementNode(node) && !node.isInline()) {
      paragraph = null
      root.append(node)
    } else {
      if (!paragraph) {
        paragraph = $createParagraphNode()
        root.append(paragraph)
      }
      paragraph.append(node)
    }
  }
  if (root.getChildrenSize() === 0) root.append($createParagraphNode())
}

export function RichTextEditor({
  html,
  onChange,
  tags,
  style,
  className,
  autoFocus,
}: {
  html: string
  onChange: (html: string) => void
  tags: string[]
  style?: CSSProperties
  className?: string
  autoFocus?: boolean
}) {
  const [initial] = useState(() => normalizeRichHtml(html))
  const config: InitialConfigType = {
    namespace: 'email-text',
    theme,
    nodes: [ListNode, ListItemNode, LinkNode, MergeTagNode],
    onError: (e) => {
      throw e
    },
    editorState: (editor) => loadHtml(editor, initial),
  }
  return (
    <LexicalComposer initialConfig={config}>
      <div className="relative">
        <RichTextPlugin
          contentEditable={
            <ContentEditable
              aria-label="Text"
              aria-placeholder="Write something…"
              placeholder={
                <div className="pointer-events-none absolute inset-x-0 top-0 opacity-50" style={style}>
                  Write something…
                </div>
              }
              className={cn('min-h-6 outline-none', className)}
              style={style}
            />
          }
          ErrorBoundary={LexicalErrorBoundary}
        />
        <Toolbar tags={tags} />
      </div>
      <HistoryPlugin />
      <ListPlugin />
      <LinkPlugin />
      <MergeTagTransformPlugin />
      {autoFocus ? <AutoFocus /> : null}
      <OnSave onChange={onChange} initial={initial} />
    </LexicalComposer>
  )
}

function AutoFocus() {
  const [editor] = useLexicalComposerContext()
  useEffect(() => editor.focus(), [editor])
  return null
}
