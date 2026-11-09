import { useState, useId, type ReactNode } from 'react'
import { useQuery } from '@apollo/client/react'
import { ArrowDownIcon, ArrowUpIcon, PackageIcon, PencilLineIcon, PlusIcon, SparklesIcon, XIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MediaPickerButton } from '@/components/MediaPicker'
import { CollectionsDocument } from '@/features/catalog/Collections.gql'
import { ProductPickerDialog } from '@/features/catalog/ProductPickerDialog'
import {
  blockDescriptor,
  blockLabel,
  SOCIAL_PLATFORMS,
  type BlockComponent,
  type CustomFieldDef,
  type EmailBlock,
  type MenuLink,
  type SocialLink,
} from './blocks'
import { BlockIcon } from './BlockIcon'
import { productImage, useBlockProducts } from './blockProducts'
import { insertAtCaret } from './caret'
import { MergeTagMenu } from './MergeTagMenu'
import { FLOATING_PANEL } from './panel'
import { plainToRich, richToPlain } from './richtext'

/** AI copy-rewrite callback (returns the rewritten text) - When absent, the
 *  "Rewrite with AI" affordance is hidden - Instruction is a preset like
 *  "make it punchier" */
export type AiRewriteFn = (text: string, instruction: string) => Promise<string>

const AI_REWRITE_PRESETS = [
  { label: 'Improve', instruction: 'improve the clarity and impact' },
  { label: 'Shorten', instruction: 'make it shorter and tighter' },
  { label: 'Make punchier', instruction: 'make it punchier and more energetic' },
  { label: 'Friendlier tone', instruction: 'make it warmer and friendlier' },
  { label: 'More formal', instruction: 'make it more formal and professional' },
]

// Merge tags suggested for a conditional-visibility field (a datalist - the
// operator can type any tag)
const VISIBILITY_TAG_SUGGESTIONS = [
  'customer.group',
  'customer.firstName',
  'customer.email',
  'order.number',
  'order.total',
]

// Custom field kinds whose value is free text and so accept {{merge.tags}}
const TAG_KINDS = new Set(['text', 'textarea', 'url', 'image'])

type Tab = 'block' | 'email' | 'theme'

/** Right-rail inspector: the selected block, the settings and (given themeSettings) the theme, as tabs */
export function BlockInspector({
  block,
  blockKey,
  onChangeBlock,
  subject,
  onSubjectChange,
  mergeTags,
  snippets,
  components,
  showSubject,
  styleControls,
  emailSettings,
  themeSettings,
  settingsLabel = 'Email',
  footer,
  aiRewrite,
  inlineText = false,
}: {
  block: EmailBlock | null
  blockKey?: string | null
  onChangeBlock: (b: EmailBlock) => void
  subject: string
  onSubjectChange: (s: string) => void
  mergeTags: string[]
  snippets: Array<{ slug: string; name: string }>
  components?: Record<string, BlockComponent>
  showSubject: boolean
  /** Per-block style controls for the selected block */
  styleControls?: ReactNode
  /** Global email style controls (shown in email settings) */
  emailSettings?: ReactNode
  themeSettings?: ReactNode
  settingsLabel?: string
  /** Pinned action bar rendered at the bottom of the panel (Save, etc.) */
  footer?: ReactNode
  /** AI copy rewrite (present only when AI is configured) */
  aiRewrite?: AiRewriteFn
  inlineText?: boolean
}) {
  // The component a `ref` points at (or a former custom-type instance whose
  // Type names a component slug)
  const component = block
    ? components?.[block.type === 'ref' ? (block.slug ?? '') : block.type]
    : undefined

  const key = blockKey !== undefined ? blockKey : block ? 'selected' : null
  const [tab, setTab] = useState<Tab>(block ? 'block' : 'email')
  const [lastKey, setLastKey] = useState(key)
  if (key !== lastKey) {
    setLastKey(key)
    setTab(key ? 'block' : tab === 'block' ? 'email' : tab)
  }

  function setField(key: keyof EmailBlock, val: string | number) {
    if (!block) return
    onChangeBlock({ ...block, [key]: val })
  }

  function setCustomField(key: string, val: string) {
    if (!block) return
    onChangeBlock({ ...block, fields: { ...(block.fields ?? {}), [key]: val } })
  }

  function setVisibleIf(v: EmailBlock['visibleIf']) {
    if (!block) return
    onChangeBlock({ ...block, visibleIf: v })
  }

  function setProducts(p: NonNullable<EmailBlock['products']>) {
    if (!block) return
    onChangeBlock({ ...block, products: p })
  }

  function setSocial(links: SocialLink[]) {
    if (!block) return
    onChangeBlock({ ...block, social: links })
  }

  const blockPanel = block ? (
    <>
      <BlockTitle block={block} component={component} />
      {block.type === 'ref' ? (
        <>
          <ContentFields block={block} setField={setField} snippets={snippets} mergeTags={mergeTags} />
          {component && component.fields.length > 0 && (
            <CustomFields fields={component.fields} values={block.fields} setCustomField={setCustomField} mergeTags={mergeTags} />
          )}
        </>
      ) : component ? (
        <CustomFields fields={component.fields} values={block.fields} setCustomField={setCustomField} mergeTags={mergeTags} />
      ) : block.type === 'products' ? (
        <ProductsFields block={block} setProducts={setProducts} />
      ) : block.type === 'social' ? (
        <SocialFields block={block} setSocial={setSocial} />
      ) : block.type === 'menu' ? (
        <MenuFields links={block.links ?? []} mergeTags={mergeTags} onChange={(links) => onChangeBlock({ ...block, links })} />
      ) : block.type === 'text' && inlineText ? (
        <TextOnCanvas block={block} onChangeBlock={onChangeBlock} aiRewrite={aiRewrite} />
      ) : (
        <>
          {block.type === 'image_text' ? <SideField side={block.side ?? 'left'} onChange={(side) => onChangeBlock({ ...block, side })} /> : null}
          <ContentFields block={block} setField={setField} snippets={snippets} mergeTags={mergeTags} aiRewrite={aiRewrite} />
        </>
      )}

      {styleControls}

      <VisibilitySection visibleIf={block.visibleIf} onChange={setVisibleIf} />
    </>
  ) : (
    <p className="text-sm text-muted-foreground">Select a block on the canvas to edit it.</p>
  )

  const emailPanel = (
    <>
      {showSubject && (
        <div className="grid gap-1.5">
          <div className="flex items-center justify-between">
            <Label htmlFor="email-subject">Subject</Label>
            <MergeTagMenu
              tags={mergeTags}
              onPick={(tag) => {
                const next = insertAtCaret('email-subject', subject, tag)
                onSubjectChange(next.value)
                next.restore()
              }}
            />
          </div>
          <Input id="email-subject" value={subject} onChange={(e) => onSubjectChange(e.target.value)} placeholder="Email subject line" />
        </div>
      )}
      {emailSettings}
    </>
  )

  return (
    <aside className={cn(FLOATING_PANEL, 'ml-0 w-72 @min-[1200px]/editor:w-80')}>
      <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)} className="flex min-h-0 flex-1 flex-col">
        <div className="border-b px-3 py-2">
          <TabsList className={cn('grid h-9 w-full', themeSettings ? 'grid-cols-3' : 'grid-cols-2')}>
            <TabsTrigger value="block" className="text-xs">
              Block
            </TabsTrigger>
            <TabsTrigger value="email" className="text-xs">
              {settingsLabel}
            </TabsTrigger>
            {themeSettings ? (
              <TabsTrigger value="theme" className="text-xs">
                Theme
              </TabsTrigger>
            ) : null}
          </TabsList>
        </div>
        <TabsContent value="block" className="mt-0 min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
          {blockPanel}
        </TabsContent>
        <TabsContent value="email" className="mt-0 min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
          {emailPanel}
        </TabsContent>
        {themeSettings ? (
          <TabsContent value="theme" className="mt-0 min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
            {themeSettings}
          </TabsContent>
        ) : null}
      </Tabs>
      {footer && <div className="shrink-0 border-t p-3">{footer}</div>}
    </aside>
  )
}

function BlockTitle({ block, component }: { block: EmailBlock; component?: BlockComponent }) {
  const hint = component ? 'A block from your reusable library.' : blockDescriptor(block.type)?.hint
  return (
    <div className="flex items-start gap-2.5">
      <span className="grid size-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground">
        <BlockIcon type={component ? 'ref' : block.type} className="size-4" />
      </span>
      <div className="min-w-0">
        <p className="text-sm font-medium">{component?.name ?? blockLabel(block.type)}</p>
        {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      </div>
    </div>
  )
}

function TextOnCanvas({ block, onChangeBlock, aiRewrite }: { block: EmailBlock; onChangeBlock: (b: EmailBlock) => void; aiRewrite?: AiRewriteFn }) {
  const plain = block.format === 'rich' ? richToPlain(block.content ?? '') : (block.content ?? '')
  return (
    <div className="flex items-start gap-2.5 rounded-md border bg-muted/30 p-3 text-xs text-muted-foreground">
      <PencilLineIcon className="mt-0.5 size-3.5 shrink-0" />
      <div className="grid flex-1 gap-2">
        <p>Write right on the canvas. Select words to make them bold, italic or a link, or start a list.</p>
        {aiRewrite ? (
          <div>
            <AiRewriteButton
              value={plain}
              rewrite={aiRewrite}
              onChange={(v) => onChangeBlock({ ...block, format: 'rich', content: plainToRich(v) })}
            />
          </div>
        ) : null}
      </div>
    </div>
  )
}

function FieldLabel({
  id,
  label,
  tags,
  value,
  onInsert,
  extra,
}: {
  id: string
  label: ReactNode
  tags?: string[]
  value: string
  onInsert: (value: string) => void
  extra?: ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <Label htmlFor={id}>{label}</Label>
      <div className="flex items-center gap-1">
        {extra}
        {tags ? (
          <MergeTagMenu
            tags={tags}
            onPick={(tag) => {
              const next = insertAtCaret(id, value, tag)
              onInsert(next.value)
              next.restore()
            }}
          />
        ) : null}
      </div>
    </div>
  )
}

function ContentFields({
  block,
  setField,
  snippets,
  mergeTags,
  aiRewrite,
}: {
  block: EmailBlock
  setField: (key: keyof EmailBlock, val: string | number) => void
  snippets: Array<{ slug: string; name: string }>
  mergeTags: string[]
  aiRewrite?: AiRewriteFn
}) {
  const descriptor = blockDescriptor(block.type)
  const fields = descriptor?.fields ?? []
  if (descriptor?.labels?.length) {
    return <BlockLabels block={block} labels={descriptor.labels} onChange={(labels) => setField('labels', labels as never)} />
  }
  if (fields.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {descriptor?.group === 'data' ? 'Fills itself in from the order.' : 'Nothing to write here - change how it looks below.'}
      </p>
    )
  }
  return (
    <div className="grid gap-3">
      {fields.map((field) => {
        const id = `f-${field.key}`
        const raw = block[field.key]
        const strVal = raw == null ? '' : String(raw)
        const tags = field.supportsTags ? mergeTags : undefined
        const set = (v: string) => setField(field.key, v)

        if (field.kind === 'ref') {
          return (
            <div key={field.key} className="grid gap-1.5">
              <Label htmlFor={id}>{field.label}</Label>
              <Select value={block.slug || undefined} onValueChange={(v) => setField('slug', v)}>
                <SelectTrigger id={id} className="w-full">
                  <SelectValue placeholder="Pick a reusable block…" />
                </SelectTrigger>
                <SelectContent>
                  {snippets.length === 0 ? (
                    <SelectItem value="__none" disabled>
                      No reusable blocks yet
                    </SelectItem>
                  ) : (
                    snippets.map((s) => (
                      <SelectItem key={s.slug} value={s.slug}>
                        {s.name} ({s.slug})
                      </SelectItem>
                    ))
                  )}
                </SelectContent>
              </Select>
            </div>
          )
        }

        if (field.kind === 'image') {
          return (
            <div key={field.key} className="grid gap-1.5">
              <FieldLabel id={id} label={field.label} tags={tags} value={strVal} onInsert={set} />
              <div className="flex items-center gap-1.5">
                <Input id={id} value={strVal} placeholder={field.placeholder} onChange={(e) => set(e.target.value)} />
                <MediaPickerButton onPick={set} />
              </div>
              {strVal && !strVal.includes('{{') && (
                <img src={strVal} alt="" className="mt-1 max-h-24 w-auto rounded border object-contain" />
              )}
            </div>
          )
        }

        const ai =
          aiRewrite && field.supportsTags && block.type !== 'html' && field.kind !== 'url' ? (
            <AiRewriteButton value={strVal} onChange={set} rewrite={aiRewrite} />
          ) : null

        if (field.kind === 'textarea') {
          return (
            <div key={field.key} className="grid gap-1.5">
              <FieldLabel id={id} label={field.label} tags={tags} value={strVal} onInsert={set} extra={ai} />
              <Textarea
                id={id}
                value={strVal}
                rows={block.type === 'html' ? 8 : 4}
                placeholder={field.placeholder}
                className={block.type === 'html' ? 'font-mono text-xs' : undefined}
                onChange={(e) => set(e.target.value)}
              />
            </div>
          )
        }

        return (
          <div key={field.key} className="grid gap-1.5">
            <FieldLabel id={id} label={field.label} tags={tags} value={strVal} onInsert={set} extra={ai} />
            <Input
              id={id}
              type={field.kind === 'number' ? 'number' : 'text'}
              value={strVal}
              placeholder={field.placeholder}
              onChange={(e) =>
                setField(field.key, field.kind === 'number' ? Number(e.target.value || 0) : e.target.value)
              }
            />
          </div>
        )
      })}
    </div>
  )
}

// AiRewriteButton offers preset copy rewrites (Improve / Shorten / …) - It calls
// the injected AI rewrite fn and replaces the field value with the result
function AiRewriteButton({
  value,
  onChange,
  rewrite,
}: {
  value: string
  onChange: (v: string) => void
  rewrite: AiRewriteFn
}) {
  const [busy, setBusy] = useState(false)
  async function run(instruction: string) {
    if (busy || !value.trim()) return
    setBusy(true)
    try {
      const out = await rewrite(value, instruction)
      if (out.trim()) onChange(out)
    } catch {
      // Surfaced by the caller's mutation error handling; swallow here
    } finally {
      setBusy(false)
    }
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={busy || !value.trim()}
          className="inline-flex items-center gap-1 rounded border bg-background px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:opacity-40"
          title="Rewrite this copy with AI"
        >
          <SparklesIcon className="size-3" />
          {busy ? 'Rewriting…' : 'AI'}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {AI_REWRITE_PRESETS.map((p) => (
          <DropdownMenuItem key={p.label} onSelect={() => void run(p.instruction)}>
            {p.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function SideField({ side, onChange }: { side: 'left' | 'right'; onChange: (side: 'left' | 'right') => void }) {
  return (
    <div className="grid gap-1.5">
      <p className="text-sm font-medium">Picture on the</p>
      <div role="radiogroup" aria-label="Picture on the" className="grid grid-cols-2 gap-1">
        {(['left', 'right'] as const).map((v) => (
          <button
            key={v}
            type="button"
            role="radio"
            aria-checked={side === v}
            onClick={() => onChange(v)}
            className={cn(
              'h-8 rounded border text-xs capitalize transition-colors',
              side === v ? 'border-primary bg-primary/10 text-primary' : 'bg-background text-muted-foreground hover:bg-accent',
            )}
          >
            {v}
          </button>
        ))}
      </div>
    </div>
  )
}

// A menu block's links in order, each a label and where it goes
function MenuFields({ links, mergeTags, onChange }: { links: MenuLink[]; mergeTags: string[]; onChange: (links: MenuLink[]) => void }) {
  const fid = useId()
  function patch(i: number, next: Partial<MenuLink>) {
    onChange(links.map((l, idx) => (idx === i ? { ...l, ...next } : l)))
  }
  function move(i: number, dir: -1 | 1) {
    const next = [...links]
    ;[next[i], next[i + dir]] = [next[i + dir], next[i]]
    onChange(next)
  }
  return (
    <div className="grid grid-cols-1 gap-2">
      <p className="text-sm font-medium">Links</p>
      {links.map((l, i) => (
        <div key={i} className="grid gap-1.5 rounded-md border p-2">
          <div className="flex items-center gap-1">
            <Input aria-label={`Link ${i + 1} label`} value={l.label} placeholder="Shop" className="h-8" onChange={(e) => patch(i, { label: e.target.value })} />
            <Button type="button" variant="ghost" size="icon" className="size-7 shrink-0" aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)}>
              <ArrowUpIcon className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-7 shrink-0" aria-label="Move down" disabled={i === links.length - 1} onClick={() => move(i, 1)}>
              <ArrowDownIcon className="size-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="icon" className="size-7 shrink-0" aria-label={`Remove ${l.label || 'link'}`} onClick={() => onChange(links.filter((_, idx) => idx !== i))}>
              <XIcon className="size-3.5" />
            </Button>
          </div>
          <div className="flex items-center gap-1">
            <Input
              id={`${fid}-url-${i}`}
              aria-label={`Link ${i + 1} address`}
              value={l.url}
              placeholder="https://… or {{storefront.url}}/sale"
              className="h-8"
              onChange={(e) => patch(i, { url: e.target.value })}
            />
            <MergeTagMenu
              tags={mergeTags}
              label=""
              onPick={(tag) => {
                const next = insertAtCaret(`${fid}-url-${i}`, l.url, tag)
                patch(i, { url: next.value })
                next.restore()
              }}
            />
          </div>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" disabled={links.length >= 12} onClick={() => onChange([...links, { label: '', url: '' }])}>
        <PlusIcon className="mr-1 size-3.5" />
        Add link
      </Button>
    </div>
  )
}

// SocialFields edits a `social` block: an ordered list of platform + URL rows
function SocialFields({
  block,
  setSocial,
}: {
  block: EmailBlock
  setSocial: (links: SocialLink[]) => void
}) {
  const links = block.social ?? []
  function patch(i: number, next: Partial<SocialLink>) {
    setSocial(links.map((l, idx) => (idx === i ? { ...l, ...next } : l)))
  }
  function add() {
    setSocial([...links, { platform: 'facebook', url: '' }])
  }
  function remove(i: number) {
    setSocial(links.filter((_, idx) => idx !== i))
  }
  return (
    <div className="grid gap-3">
      <Label>Social profiles</Label>
      <div className="grid gap-2">
        {links.map((l, i) => (
          <div key={i} className="flex items-center gap-1.5">
            <Select value={l.platform} onValueChange={(v) => patch(i, { platform: v })}>
              <SelectTrigger className="w-32 shrink-0 capitalize">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SOCIAL_PLATFORMS.map((p) => (
                  <SelectItem key={p} value={p} className="capitalize">
                    {p === 'x' ? 'X' : p}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              value={l.url}
              placeholder="https://…"
              onChange={(e) => patch(i, { url: e.target.value })}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="shrink-0"
              aria-label="Remove link"
              onClick={() => remove(i)}
            >
              <XIcon className="size-4" />
            </Button>
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={add}>
          <PlusIcon className="mr-1 size-3.5" />
          Add profile
        </Button>
      </div>
    </div>
  )
}

// CustomFields renders the schema-driven inputs for a parameterized block's field values (stored in block.fields)
function CustomFields({
  fields,
  values,
  setCustomField,
  mergeTags,
}: {
  fields: CustomFieldDef[]
  values?: Record<string, string>
  setCustomField: (key: string, val: string) => void
  mergeTags: string[]
}) {
  if (fields.length === 0) {
    return <p className="text-sm text-muted-foreground">This block has no inputs.</p>
  }
  return (
    <div className="grid gap-3">
      {fields.map((field) => (
        <CustomFieldInput
          key={field.key}
          field={field}
          value={values?.[field.key] ?? ''}
          onChange={(v) => setCustomField(field.key, v)}
          mergeTags={mergeTags}
        />
      ))}
    </div>
  )
}

function CustomFieldInput({
  field,
  value,
  onChange,
  mergeTags,
}: {
  field: CustomFieldDef
  value: string
  onChange: (v: string) => void
  mergeTags: string[]
}) {
  const id = `cf-${field.key}`
  const name = (
    <>
      {field.label || field.key}
      {field.required && <span className="ml-0.5 text-destructive">*</span>}
    </>
  )
  const label = TAG_KINDS.has(field.kind) ? (
    <FieldLabel id={id} label={name} tags={mergeTags} value={value} onInsert={onChange} />
  ) : (
    <Label htmlFor={id}>{name}</Label>
  )

  if (field.kind === 'textarea') {
    return (
      <div className="grid gap-1.5">
        {label}
        <Textarea id={id} rows={4} value={value} onChange={(e) => onChange(e.target.value)} />
      </div>
    )
  }

  if (field.kind === 'select') {
    return (
      <div className="grid gap-1.5">
        {label}
        <Select value={value || undefined} onValueChange={onChange}>
          <SelectTrigger id={id} className="w-full">
            <SelectValue placeholder="Choose…" />
          </SelectTrigger>
          <SelectContent>
            {(field.options ?? []).length === 0 ? (
              <SelectItem value="__none" disabled>
                No options defined
              </SelectItem>
            ) : (
              (field.options ?? []).map((opt) => (
                <SelectItem key={opt} value={opt}>
                  {opt}
                </SelectItem>
              ))
            )}
          </SelectContent>
        </Select>
      </div>
    )
  }

  if (field.kind === 'boolean') {
    return (
      <label className="flex items-center gap-2 text-sm" htmlFor={id}>
        <input
          id={id}
          type="checkbox"
          className="size-4"
          checked={value === 'true'}
          onChange={(e) => onChange(e.target.checked ? 'true' : 'false')}
        />
        {field.label || field.key}
      </label>
    )
  }

  if (field.kind === 'color') {
    return (
      <div className="grid gap-1.5">
        {label}
        <div className="flex items-center gap-2">
          <input
            type="color"
            className="h-9 w-12 cursor-pointer rounded border"
            value={value || '#000000'}
            onChange={(e) => onChange(e.target.value)}
          />
          <Input id={id} value={value} placeholder="#000000" onChange={(e) => onChange(e.target.value)} />
        </div>
      </div>
    )
  }

  const inputType = field.kind === 'number' ? 'number' : field.kind === 'date' ? 'date' : 'text'
  return (
    <div className="grid gap-1.5">
      {label}
      <Input
        id={id}
        type={inputType}
        value={value}
        placeholder={field.kind === 'url' || field.kind === 'image' ? 'https://…' : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  )
}

// ProductsFields configures a `products` block: source mode (specific SKUs or a
// collection), the SKU list / collection picker, and layout (limit + columns)
function ProductsFields({
  block,
  setProducts,
}: {
  block: EmailBlock
  setProducts: (p: NonNullable<EmailBlock['products']>) => void
}) {
  const fid = useId()
  const p = block.products ?? { mode: 'skus', skus: [], limit: 3, columns: 3 }
  const { data: collectionsData } = useQuery(CollectionsDocument, {
    skip: p.mode !== 'collection',
    fetchPolicy: 'cache-first',
  })
  const collections = collectionsData?.collections ?? []
  const skus = p.skus ?? []

  function patch(next: Partial<NonNullable<EmailBlock['products']>>) {
    setProducts({ ...p, ...next })
  }

  return (
    <div className="grid grid-cols-1 gap-3">
      <div className="grid gap-1.5">
        <Label htmlFor={`${fid}-source`}>Source</Label>
        <Select value={p.mode || 'skus'} onValueChange={(v) => patch({ mode: v })}>
          <SelectTrigger id={`${fid}-source`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="skus">Specific products</SelectItem>
            <SelectItem value="collection">Collection</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {p.mode === 'collection' ? (
        <div className="grid gap-1.5">
          <Label htmlFor={`${fid}-collection`}>Collection</Label>
          {collections.length > 0 ? (
            <Select value={p.collectionId || undefined} onValueChange={(v) => patch({ collectionId: v })}>
              <SelectTrigger id={`${fid}-collection`} className="w-full">
                <SelectValue placeholder="Pick a collection…" />
              </SelectTrigger>
              <SelectContent>
                {collections.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name || c.slug}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              id={`${fid}-collection`}
              value={p.collectionId ?? ''}
              placeholder="Collection ID"
              onChange={(e) => patch({ collectionId: e.target.value })}
            />
          )}
        </div>
      ) : (
        <PickedProducts
          skus={skus}
          limit={p.limit && p.limit > 0 ? p.limit : 3}
          // Raise the limit only for additions; reordering and removal keep the explicit cap.
          onChange={(next) => patch({
            skus: next,
            ...(next.length > skus.length ? { limit: Math.max(p.limit && p.limit > 0 ? p.limit : 3, next.length) } : {}),
          })}
        />
      )}

      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-1.5">
          <Label htmlFor="products-limit">Max products</Label>
          <Input
            id="products-limit"
            type="number"
            min={1}
            value={p.limit ?? ''}
            placeholder="3"
            onChange={(e) => patch({ limit: Number(e.target.value || 0) })}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="products-columns">Columns</Label>
          <Input
            id="products-columns"
            type="number"
            min={1}
            max={4}
            value={p.columns ?? ''}
            placeholder="3"
            onChange={(e) => patch({ columns: Number(e.target.value || 0) })}
          />
        </div>
      </div>
    </div>
  )
}

function PickedProducts({ skus, limit, onChange }: { skus: string[]; limit: number; onChange: (skus: string[]) => void }) {
  const [picking, setPicking] = useState(false)
  const { bySku, loading } = useBlockProducts(skus)
  const taken = new Set(skus.map((s) => bySku.get(s)?.id).filter((id): id is string => !!id))
  function move(i: number, dir: -1 | 1) {
    const next = [...skus]
    ;[next[i], next[i + dir]] = [next[i + dir], next[i]]
    onChange(next)
  }
  return (
    <div className="grid grid-cols-1 gap-1.5">
      <p className="text-sm font-medium">Products</p>
      {skus.length === 0 ? (
        <p className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">No products yet.</p>
      ) : (
        <ul className="divide-y rounded-md border">
          {skus.map((sku, i) => {
            const product = bySku.get(sku)
            const image = productImage(product)
            return (
              <li key={sku} className={cn('flex items-center gap-2 px-2 py-1.5', i >= limit && 'opacity-50')}>
                {image ? (
                  <img src={image} alt="" className="size-9 shrink-0 rounded border object-cover" />
                ) : (
                  <span className="grid size-9 shrink-0 place-items-center rounded border bg-muted">
                    <PackageIcon className="size-4 text-muted-foreground" />
                  </span>
                )}
                <div className="min-w-0 flex-1">
                  {product ? (
                    <p className="truncate text-sm">{product.name}</p>
                  ) : loading ? (
                    <p className="truncate text-sm text-muted-foreground">Loading…</p>
                  ) : (
                    <p className="truncate text-sm text-warning">Not in the catalog</p>
                  )}
                  <p className="truncate font-mono text-[11px] text-muted-foreground">{sku}</p>
                  {i >= limit ? <p className="text-[11px] text-warning">Past Max products - not sent</p> : null}
                </div>
                <div className="flex shrink-0 items-center">
                  <Button type="button" variant="ghost" size="icon" className="size-7" aria-label="Move up" disabled={i === 0} onClick={() => move(i, -1)}>
                    <ArrowUpIcon className="size-3.5" />
                  </Button>
                  <Button type="button" variant="ghost" size="icon" className="size-7" aria-label="Move down" disabled={i === skus.length - 1} onClick={() => move(i, 1)}>
                    <ArrowDownIcon className="size-3.5" />
                  </Button>
                  <Button type="button" variant="ghost" size="icon" className="size-7" aria-label={`Remove ${product?.name ?? sku}`} onClick={() => onChange(skus.filter((s) => s !== sku))}>
                    <XIcon className="size-3.5" />
                  </Button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
      <Button type="button" variant="outline" size="sm" onClick={() => setPicking(true)}>
        <PlusIcon className="mr-1 size-3.5" />
        Choose products
      </Button>
      <ProductPickerDialog
        open={picking}
        onOpenChange={setPicking}
        title="Products for this email"
        description="Ticked products join the end of the block, and show with their live price when the email is sent."
        takenIds={taken}
        takenLabel="In this block"
        confirmLabel={(n) => (n === 0 ? 'Add products' : n === 1 ? 'Add 1 product' : `Add ${n} products`)}
        onConfirm={(picks) => {
          const added = picks.map((pick) => pick.product.sku).filter((sku) => sku && !skus.includes(sku))
          onChange([...skus, ...added])
        }}
      />
    </div>
  )
}

// VisibilitySection adds a conditional-visibility rule to any block - Toggling it
// off clears `visibleIf`
function VisibilitySection({
  visibleIf,
  onChange,
}: {
  visibleIf: EmailBlock['visibleIf']
  onChange: (v: EmailBlock['visibleIf']) => void
}) {
  const enabled = !!visibleIf
  const rule = visibleIf ?? { field: '', op: 'exists', value: '', negate: false }

  function toggle(on: boolean) {
    onChange(on ? { field: '', op: 'exists', value: '', negate: false } : undefined)
  }
  function patch(next: Partial<NonNullable<EmailBlock['visibleIf']>>) {
    onChange({ ...rule, ...next })
  }

  return (
    <div className="space-y-3 border-t pt-4">
      <div className="flex items-center justify-between">
        <Label htmlFor="vis-toggle" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Visibility
        </Label>
      </div>
      <label className="flex items-center gap-2 text-sm" htmlFor="vis-toggle">
        <Switch id="vis-toggle" checked={enabled} onCheckedChange={toggle} />
        Only show this block if…
      </label>

      {enabled && (
        <div className="grid gap-2.5 rounded-md border bg-muted/30 p-2.5">
          <div className="grid gap-1.5">
            <Label htmlFor="vis-field">Merge-tag field</Label>
            <Input
              id="vis-field"
              list="vis-field-suggestions"
              value={rule.field}
              placeholder="customer.group"
              onChange={(e) => patch({ field: e.target.value })}
            />
            <datalist id="vis-field-suggestions">
              {VISIBILITY_TAG_SUGGESTIONS.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="vis-op">Condition</Label>
            <Select value={rule.op || 'exists'} onValueChange={(v) => patch({ op: v })}>
              <SelectTrigger id="vis-op" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="exists">exists</SelectItem>
                <SelectItem value="eq">equals</SelectItem>
                <SelectItem value="ne">not equals</SelectItem>
                <SelectItem value="contains">contains</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {rule.op !== 'exists' && (
            <div className="grid gap-1.5">
              <Label htmlFor="vis-value">Value</Label>
              <Input
                id="vis-value"
                value={rule.value ?? ''}
                placeholder="wholesale"
                onChange={(e) => patch({ value: e.target.value })}
              />
            </div>
          )}
          <label className="flex items-center gap-2 text-sm" htmlFor="vis-negate">
            <Switch
              id="vis-negate"
              checked={!!rule.negate}
              onCheckedChange={(on) => patch({ negate: on })}
            />
            Negate (show when the condition is NOT met)
          </label>
        </div>
      )}
    </div>
  )
}

// An order block fills itself from the order, so what's left to write is its words - each language's version of the
// email says them its own way, and an empty one keeps the English default
function BlockLabels({
  block,
  labels,
  onChange,
}: {
  block: EmailBlock
  labels: NonNullable<ReturnType<typeof blockDescriptor>>['labels'] & object
  onChange: (labels: Record<string, string>) => void
}) {
  const current = block.labels ?? {}
  return (
    <div className="grid gap-3">
      <p className="text-xs text-muted-foreground">Fills in from the order. Change the words it prints here - empty keeps the default.</p>
      {labels.map((l) => (
        <div key={l.key} className="grid gap-1.5">
          <Label htmlFor={`label-${l.key}`}>{l.label}</Label>
          <Input
            id={`label-${l.key}`}
            value={current[l.key] ?? ''}
            maxLength={60}
            placeholder={l.fallback}
            onChange={(e) => {
              const next = { ...current, [l.key]: e.target.value }
              if (!e.target.value) delete next[l.key]
              onChange(next)
            }}
          />
        </div>
      ))}
    </div>
  )
}
