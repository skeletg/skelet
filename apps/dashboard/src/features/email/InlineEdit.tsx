import { useId, useLayoutEffect, useRef, useState } from 'react'
import { buttonStyle, headingStyle, RICH_TEXT_CLASSES, textBlockStyle, type EmailBlock, type EmailStyles } from './blocks'
import { insertAtCaret } from './caret'
import { MergeTagMenu } from './MergeTagMenu'
import { RichTextEditor } from './RichTextEditor'
import { plainToRich } from './richtext'

type Props = { block: EmailBlock; theme: EmailStyles; tags: string[]; onChange: (b: EmailBlock) => void }

export function InlineEditor(props: Props) {
  switch (props.block.type) {
    case 'heading':
      return <InlineHeading {...props} />
    case 'button':
      return <InlineButton {...props} />
    case 'text':
      return <InlineText {...props} />
    default:
      return null
  }
}

function VariableBar({ tags, onPick }: { tags: string[]; onPick: (tag: string) => void }) {
  if (tags.length === 0) return null
  return (
    <div className="absolute left-0 top-full z-30 mt-2 rounded-md border bg-background p-0.5 font-sans text-foreground shadow-md" onClick={(e) => e.stopPropagation()}>
      <MergeTagMenu tags={tags} onPick={onPick} label="Insert variable" />
    </div>
  )
}

function InlineHeading({ block, theme, tags, onChange }: Props) {
  const id = useId()
  const ref = useRef<HTMLTextAreaElement>(null)
  const value = block.text ?? ''
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [value, block.style?.fontSize])
  useLayoutEffect(() => ref.current?.focus({ preventScroll: true }), [])
  return (
    <div className="relative">
      <textarea
        id={id}
        ref={ref}
        aria-label="Heading text"
        rows={1}
        value={value}
        placeholder="Heading"
        onChange={(e) => onChange({ ...block, text: e.target.value.replace(/\n/g, ' ') })}
        onKeyDown={(e) => e.key === 'Enter' && e.preventDefault()}
        className="block w-full resize-none overflow-hidden border-0 bg-transparent p-0 outline-none placeholder:opacity-50"
        style={{ ...headingStyle(block, theme), fontFamily: 'inherit' }}
      />
      <VariableBar
        tags={tags}
        onPick={(tag) => {
          const next = insertAtCaret(id, value, tag)
          onChange({ ...block, text: next.value })
          next.restore()
        }}
      />
    </div>
  )
}

function InlineButton({ block, theme, tags, onChange }: Props) {
  const id = useId()
  const ref = useRef<HTMLInputElement>(null)
  const value = block.label ?? ''
  useLayoutEffect(() => ref.current?.focus({ preventScroll: true }), [])
  return (
    <div className="relative" style={{ textAlign: block.style?.align ?? 'left', margin: '16px 0' }}>
      <span style={buttonStyle(block, theme)}>
        <input
          id={id}
          ref={ref}
          aria-label="Button label"
          value={value}
          placeholder="Button"
          size={Math.max(6, value.length + 1)}
          onChange={(e) => onChange({ ...block, label: e.target.value })}
          className="border-0 bg-transparent p-0 text-center outline-none placeholder:text-current placeholder:opacity-60"
          style={{ color: 'inherit', font: 'inherit' }}
        />
      </span>
      <VariableBar
        tags={tags}
        onPick={(tag) => {
          const next = insertAtCaret(id, value, tag)
          onChange({ ...block, label: next.value })
          next.restore()
        }}
      />
    </div>
  )
}

function InlineText({ block, theme, tags, onChange }: Props) {
  const content = block.content ?? ''
  const html = block.format === 'rich' ? content : plainToRich(content)
  // The editor owns its words while it's open. Content that changes under it (undo, an AI rewrite) opens it afresh;
  // its own edits, echoed back, don't
  const [seen, setSeen] = useState(content)
  const [epoch, setEpoch] = useState(0)
  if (content !== seen) {
    setSeen(content)
    setEpoch((e) => e + 1)
  }
  return (
    <div style={{ margin: '0 0 12px' }}>
      <RichTextEditor
        key={epoch}
        html={html}
        tags={tags}
        autoFocus
        className={RICH_TEXT_CLASSES}
        style={{ ...textBlockStyle(block, theme), ['--email-link' as string]: theme.linkColor || undefined }}
        onChange={(next) => {
          setSeen(next)
          onChange({ ...block, format: 'rich', content: next })
        }}
      />
    </div>
  )
}
