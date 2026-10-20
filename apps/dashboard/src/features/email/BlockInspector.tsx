import { useRef, useState, useId } from 'react'
import { useQuery } from '@apollo/client/react'
import { PlusIcon, SparklesIcon, XIcon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
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
import {
  blockDescriptor,
  blockLabel,
  SOCIAL_PLATFORMS,
  type BlockComponent,
  type CustomFieldDef,
  type EmailBlock,
  type SocialLink,
} from './blocks'

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

/** Right-rail inspector - Edits the selected block's content (+ merge-tag
 *  insertion); with no block selected it shows email settings (subject) - A
 *  `ref` to a parameterized component renders the component picker plus a
 *  schema-driven form for the component's fields - Style controls layer in via
 *  the `styleControls` slot */
export function BlockInspector({
  block,
  onChangeBlock,
  subject,
  onSubjectChange,
  mergeTags,
  snippets,
  components,
  showSubject,
  styleControls,
  emailSettings,
  footer,
  aiRewrite,
}: {
  block: EmailBlock | null
  onChangeBlock: (b: EmailBlock) => void
  subject: string
  onSubjectChange: (s: string) => void
  mergeTags: string[]
  snippets: Array<{ slug: string; name: string }>
  components?: Record<string, BlockComponent>
  showSubject: boolean
  /** Per-block style controls for the selected block */
  styleControls?: React.ReactNode
  /** Global email style controls (shown in email settings) */
  emailSettings?: React.ReactNode
  /** Pinned action bar rendered at the bottom of the panel (Save, etc.) */
  footer?: React.ReactNode
  /** AI copy rewrite (present only when AI is configured) */
  aiRewrite?: AiRewriteFn
}) {
  const activeRef = useRef<HTMLInputElement | HTMLTextAreaElement | null>(null)
  // The component a `ref` points at (or a former custom-type instance whose
  // Type names a component slug)
  const component = block
    ? components?.[block.type === 'ref' ? (block.slug ?? '') : block.type]
    : undefined

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

  function insertTag(tag: string) {
    const el = activeRef.current
    const token = `{{${tag}}}`
    if (!el) return
    const key = el.dataset.field
    if (!key) return
    const isCustom = el.dataset.custom === '1'
    const cur = isCustom
      ? String(block?.fields?.[key] ?? '')
      : String(block?.[key as keyof EmailBlock] ?? '')
    const start = el.selectionStart ?? cur.length
    const end = el.selectionEnd ?? cur.length
    const next = cur.slice(0, start) + token + cur.slice(end)
    if (isCustom) setCustomField(key, next)
    else setField(key as keyof EmailBlock, next)
    const caret = start + token.length
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(caret, caret)
    })
  }

  const showTags = !!block && mergeTags.length > 0 && hasTagField(block, component)

  return (
    <aside className="flex w-80 shrink-0 flex-col border-l bg-card">
      <div className="border-b px-4 py-3">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {block ? `Edit ${component?.name ?? blockLabel(block.type)}` : 'Email settings'}
        </h2>
      </div>

      <div className="flex-1 space-y-5 overflow-y-auto p-4">
        {!block && (
          <>
            {showSubject && (
              <div className="grid gap-1.5">
                <Label htmlFor="email-subject">Subject</Label>
                <Input
                  id="email-subject"
                  value={subject}
                  onChange={(e) => onSubjectChange(e.target.value)}
                  placeholder="Email subject line"
                />
                <p className="text-xs text-muted-foreground">Supports merge tags, e.g. {`{{order.number}}`}.</p>
              </div>
            )}
            {emailSettings}
            <p className="text-xs text-muted-foreground">Select a block on the canvas to edit it.</p>
          </>
        )}

        {block && (
          <>
            {block.type === 'ref' ? (
              <>
                <ContentFields block={block} setField={setField} activeRef={activeRef} snippets={snippets} />
                {component && component.fields.length > 0 && (
                  <CustomFields
                    fields={component.fields}
                    values={block.fields}
                    setCustomField={setCustomField}
                    activeRef={activeRef}
                  />
                )}
              </>
            ) : component ? (
              <CustomFields
                fields={component.fields}
                values={block.fields}
                setCustomField={setCustomField}
                activeRef={activeRef}
              />
            ) : block.type === 'products' ? (
              <ProductsFields block={block} setProducts={setProducts} />
            ) : block.type === 'social' ? (
              <SocialFields block={block} setSocial={setSocial} />
            ) : (
              <ContentFields block={block} setField={setField} activeRef={activeRef} snippets={snippets} aiRewrite={aiRewrite} />
            )}

            {showTags && (
              <div className="rounded-md border bg-muted/30 p-2.5">
                <p className="mb-1.5 text-[11px] font-medium text-muted-foreground">
                  Insert merge tag (into the focused field)
                </p>
                <div className="flex flex-wrap gap-1">
                  {mergeTags.map((tag) => (
                    <button
                      key={tag}
                      type="button"
                      onClick={() => insertTag(tag)}
                      className="rounded border bg-background px-1.5 py-0.5 font-mono text-[10px] hover:bg-accent hover:text-accent-foreground"
                    >
                      {`{{${tag}}}`}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {styleControls}

            <VisibilitySection visibleIf={block.visibleIf} onChange={setVisibleIf} />
          </>
        )}
      </div>

      {footer && <div className="shrink-0 border-t p-3">{footer}</div>}
    </aside>
  )
}

function hasTagField(block: EmailBlock, component: BlockComponent | undefined): boolean {
  if (component) return component.fields.some((f) => TAG_KINDS.has(f.kind))
  return (blockDescriptor(block.type)?.fields ?? []).some((f) => f.supportsTags)
}

function ContentFields({
  block,
  setField,
  activeRef,
  snippets,
  aiRewrite,
}: {
  block: EmailBlock
  setField: (key: keyof EmailBlock, val: string | number) => void
  activeRef: React.MutableRefObject<HTMLInputElement | HTMLTextAreaElement | null>
  snippets: Array<{ slug: string; name: string }>
  aiRewrite?: AiRewriteFn
}) {
  const fields = blockDescriptor(block.type)?.fields ?? []
  if (fields.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        This block has no content settings - it renders from the order automatically.
      </p>
    )
  }
  return (
    <div className="grid gap-3">
      {fields.map((field) => {
        const id = `f-${field.key}`
        const raw = block[field.key]
        const strVal = raw == null ? '' : String(raw)

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
              <Label htmlFor={id}>{field.label}</Label>
              <div className="flex items-center gap-1.5">
                <Input
                  id={id}
                  data-field={field.key}
                  value={strVal}
                  placeholder={field.placeholder}
                  onFocus={(e) => field.supportsTags && (activeRef.current = e.currentTarget)}
                  onChange={(e) => setField(field.key, e.target.value)}
                />
                <MediaPickerButton onPick={(url) => setField(field.key, url)} />
              </div>
              {strVal && !strVal.includes('{{') && (
                <img src={strVal} alt="" className="mt-1 max-h-24 w-auto rounded border object-contain" />
              )}
            </div>
          )
        }

        if (field.kind === 'textarea') {
          return (
            <div key={field.key} className="grid gap-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor={id}>{field.label}</Label>
                {aiRewrite && field.supportsTags && block.type !== 'html' && (
                  <AiRewriteButton
                    value={strVal}
                    onChange={(v) => setField(field.key, v)}
                    rewrite={aiRewrite}
                  />
                )}
              </div>
              <Textarea
                id={id}
                data-field={field.key}
                value={strVal}
                rows={4}
                placeholder={field.placeholder}
                onFocus={(e) => field.supportsTags && (activeRef.current = e.currentTarget)}
                onChange={(e) => setField(field.key, e.target.value)}
              />
            </div>
          )
        }

        return (
          <div key={field.key} className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor={id}>{field.label}</Label>
              {aiRewrite && field.supportsTags && field.kind === 'text' && (
                <AiRewriteButton
                  value={strVal}
                  onChange={(v) => setField(field.key, v)}
                  rewrite={aiRewrite}
                />
              )}
            </div>
            <Input
              id={id}
              data-field={field.key}
              type={field.kind === 'number' ? 'number' : 'text'}
              value={strVal}
              placeholder={field.placeholder}
              onFocus={(e) => field.supportsTags && (activeRef.current = e.currentTarget)}
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

// CustomFields renders the editing form for a custom block-type instance, driven entirely by the type's `fields` schema - Values live in
// block.fields CustomFields renders the schema-driven inputs for a parameterized block's field values (stored in block.fields)
function CustomFields({
  fields,
  values,
  setCustomField,
  activeRef,
}: {
  fields: CustomFieldDef[]
  values?: Record<string, string>
  setCustomField: (key: string, val: string) => void
  activeRef: React.MutableRefObject<HTMLInputElement | HTMLTextAreaElement | null>
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
          activeRef={activeRef}
        />
      ))}
    </div>
  )
}

function CustomFieldInput({
  field,
  value,
  onChange,
  activeRef,
}: {
  field: CustomFieldDef
  value: string
  onChange: (v: string) => void
  activeRef: React.MutableRefObject<HTMLInputElement | HTMLTextAreaElement | null>
}) {
  const id = `cf-${field.key}`
  const tagged = TAG_KINDS.has(field.kind)
  const label = (
    <Label htmlFor={id}>
      {field.label || field.key}
      {field.required && <span className="ml-0.5 text-destructive">*</span>}
    </Label>
  )

  if (field.kind === 'textarea') {
    return (
      <div className="grid gap-1.5">
        {label}
        <Textarea
          id={id}
          data-field={field.key}
          data-custom="1"
          rows={4}
          value={value}
          onFocus={(e) => tagged && (activeRef.current = e.currentTarget)}
          onChange={(e) => onChange(e.target.value)}
        />
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
          <Input value={value} placeholder="#000000" onChange={(e) => onChange(e.target.value)} />
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
        data-field={field.key}
        data-custom="1"
        type={inputType}
        value={value}
        placeholder={field.kind === 'url' || field.kind === 'image' ? 'https://…' : undefined}
        onFocus={(e) => tagged && (activeRef.current = e.currentTarget)}
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
  function setSku(i: number, val: string) {
    const arr = [...skus]
    arr[i] = val
    patch({ skus: arr })
  }
  function addSku() {
    patch({ skus: [...skus, ''] })
  }
  function removeSku(i: number) {
    patch({ skus: skus.filter((_, idx) => idx !== i) })
  }

  return (
    <div className="grid gap-3">
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
        <div className="grid gap-1.5">
          <Label>SKUs</Label>
          <div className="grid gap-1.5">
            {skus.map((sku, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <Input value={sku} placeholder="SKU-123" onChange={(e) => setSku(i, e.target.value)} />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="shrink-0"
                  aria-label="Remove SKU"
                  onClick={() => removeSku(i)}
                >
                  <XIcon className="size-4" />
                </Button>
              </div>
            ))}
            <Button type="button" variant="outline" size="sm" onClick={addSku}>
              <PlusIcon className="mr-1 size-3.5" />
              Add SKU
            </Button>
          </div>
        </div>
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
