import { useEffect, useMemo, useState, useId } from 'react'
import { useQuery } from '@apollo/client/react'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDownIcon, ChevronRightIcon, ChevronUpIcon, GripVerticalIcon, PlusIcon, SearchIcon, Trash2Icon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { AnimatedPanel } from './AnimatedPanel'
import { holdInPlace } from './motion'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CollectionsDocument } from '@/features/catalog/Collections.gql'
import { WebFormsDocument } from '@/features/webforms/WebForms.gql'
import { useConfigScope } from '@/lib/config-scope'
import { useCategoryIndex } from '@/features/catalog/category-picker/category-options'
import { CategorySelect } from '@/features/catalog/category-picker/CategorySelect'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import { MediaImageInput } from '@/features/media/MediaImageInput'
import { type AssetRef } from '@/features/media/asset-ref'
import { PrismSegmentsDocument } from '@/features/prism/Prism.gql'
import { useFeatures } from '@/shared/features/useFeatures'
import { LexicalRichTextEditor, type RichTextValue } from '@/features/catalog/LexicalRichTextEditor'
import { LinkField } from './LinkField'
import {
  BLOCK_CATEGORY_ORDER,
  CMS_BLOCK_TYPES,
  blockCategory,
  blockDescriptor,
  blockLabel,
  newBlock,
  registerCustomBlockTypes,
  registryOptions,
  shortPreview,
  type CmsBlock,
  type CmsBlockField,
} from './page-blocks'
import { CmsBlockTypesDocument, CmsSnippetsDocument } from './Cms.gql'

// PageBlockCanvas is the CMS page-body builder: a single vertical column of
// block cards that expand in place to edit their fields (accordion), with an
// "Add block" menu beneath - Drag a card's handle to reorder - FLAT (v1 = an
// ordered list, no nested containers) - Controlled - the parent owns `blocks`
// so it can stage/serialize the Document; every mutation reports via onChange

// ---------- entity pickers (collection / category) ----------

function EntityPicker({
  kind,
  value,
  onChange,
  disabled,
}: {
  kind: 'collection' | 'category'
  value: string
  onChange: (v: string | undefined) => void
  disabled: boolean
}) {
  return kind === 'category' ? (
    <CategoryEntityPicker value={value} onChange={onChange} disabled={disabled} />
  ) : (
    <CollectionPicker value={value} onChange={onChange} disabled={disabled} />
  )
}

// The shared category tree - browse or search by name, each option labelled by its path
function CategoryEntityPicker({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string | undefined) => void
  disabled: boolean
}) {
  const { error } = useCategoryIndex()
  // If the catalog query fails (e.g. the admin lacks catalog read), degrade to a
  // raw id input rather than an empty, useless picker
  if (error) {
    return (
      <Input value={value} placeholder="category id" disabled={disabled} onChange={(e) => onChange(e.target.value || undefined)} />
    )
  }
  return (
    <CategorySelect
      value={value}
      onChange={(id) => onChange(id || undefined)}
      disabled={disabled}
      placeholder="Search categories…"
      aria-label="Category"
    />
  )
}

function CollectionPicker({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string | undefined) => void
  disabled: boolean
}) {
  const collections = useQuery(CollectionsDocument, { fetchPolicy: 'cache-first' })

  const options = useMemo<{ value: string; label: string; hint?: string }[]>(
    () =>
      (collections.data?.collections ?? []).map((c) => {
        const n = c.products.length
        return { value: c.id, label: c.name || c.slug, hint: `${n} product${n === 1 ? '' : 's'} · ${c.id.slice(0, 8)}` }
      }),
    [collections.data],
  )

  // If the catalog query fails (e.g. the admin lacks catalog read), degrade to a
  // raw id input rather than an empty, useless dropdown
  if (collections.error) {
    return (
      <Input
        value={value}
        placeholder="collection id"
        disabled={disabled}
        onChange={(e) => onChange(e.target.value || undefined)}
      />
    )
  }
  // Searchable dropdown - type to filter by name, so it scales to large catalogs.
  // Each option carries a short id and product count as the hint, so similarly-named
  // entries stay distinguishable
  return (
    <AutocompletePicker
      options={options}
      value={value || ''}
      onChange={(v) => onChange((typeof v === 'string' ? v : v[0]) || undefined)}
      placeholder={collections.loading ? 'Loading…' : 'Search collections…'}
      emptyMessage="No collections found."
      loading={collections.loading}
    />
  )
}

// FormPicker selects a custom web form (for the `form` block), scoped to the
// active channel
function FormPicker({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string | undefined) => void
  disabled: boolean
}) {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(WebFormsDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-first',
  })
  const options = useMemo(
    () =>
      (data?.webForms ?? []).map((f) => ({
        value: f.id,
        label: f.name || f.slug,
        hint: f.status === 'active' ? 'active' : 'draft',
      })),
    [data],
  )
  if (error) {
    return <Input value={value} placeholder="form id" disabled={disabled} onChange={(e) => onChange(e.target.value || undefined)} />
  }
  return (
    <AutocompletePicker
      options={options}
      value={value || ''}
      onChange={(v) => onChange((typeof v === 'string' ? v : v[0]) || undefined)}
      placeholder={loading ? 'Loading…' : 'Search forms…'}
      emptyMessage="No forms yet - create one under CMS → Forms."
      loading={loading}
    />
  )
}

// ---------- one field ----------

function FieldInput({
  field,
  value,
  onChange,
  disabled,
}: {
  field: CmsBlockField
  value: unknown
  onChange: (v: unknown) => void
  disabled: boolean
}) {
  const id = `cms-f-${field.key}`
  const str = typeof value === 'string' ? value : ''

  let control: React.ReactNode
  switch (field.kind) {
    case 'link':
      control = <LinkField value={value} onChange={onChange} disabled={disabled} />
      break
    case 'variant': {
      const opts = registryOptions(field.registry)
      if (field.searchable) {
        // Extensible lists (card / section styles) - a searchable dropdown that
        // scales past a handful of options
        control = (
          <AutocompletePicker
            options={opts.map((o) => ({ value: o.id, label: o.label, hint: o.description }))}
            value={str || opts[0]?.id || ''}
            onChange={(v) => onChange(typeof v === 'string' ? v : v[0])}
            placeholder="Choose…"
            emptyMessage="No styles registered."
          />
        )
        break
      }
      const active = str || opts[0]?.id
      control = (
        <div className="grid grid-cols-3 gap-2">
          {opts.map((o) => (
            <button
              key={o.id}
              type="button"
              disabled={disabled}
              onClick={() => onChange(o.id)}
              title={o.description}
              className={cn(
                'rounded-md border p-2 text-left transition-colors',
                active === o.id ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:bg-accent',
              )}
            >
              <span className="block text-xs font-medium">{o.label}</span>
              {o.description && <span className="mt-0.5 block text-[10px] leading-tight text-muted-foreground">{o.description}</span>}
            </button>
          ))}
        </div>
      )
      break
    }
    case 'textarea':
      control = (
        <Textarea
          id={id}
          rows={field.key === 'html' ? 6 : 3}
          className={field.key === 'html' ? 'font-mono text-xs' : undefined}
          value={str}
          placeholder={field.placeholder}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
        />
      )
      break
    case 'select':
      control = (
        <Select value={str || undefined} onValueChange={(v) => onChange(v)} disabled={disabled}>
          <SelectTrigger id={id} className="w-full">
            <SelectValue placeholder="Choose…" />
          </SelectTrigger>
          <SelectContent>
            {(field.options ?? []).map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )
      break
    case 'number':
      control = (
        <Input
          id={id}
          type="number"
          min={1}
          value={typeof value === 'number' ? String(value) : ''}
          placeholder={field.placeholder}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
        />
      )
      break
    case 'image':
      control = (
        <MediaImageInput
          value={value as string | AssetRef | undefined}
          onChange={(v) => onChange(v ?? undefined)}
          disabled={disabled}
          placeholder={field.placeholder}
          path="cms/pages"
        />
      )
      break
    case 'collection':
    case 'category':
      control = <EntityPicker kind={field.kind} value={str} onChange={onChange} disabled={disabled} />
      break
    case 'form':
      control = <FormPicker value={str} onChange={onChange} disabled={disabled} />
      break
    case 'richtext':
      control = (
        <LexicalRichTextEditor
          value={(value as RichTextValue) ?? null}
          fallbackText=""
          disabled={disabled}
          onChange={(v) => onChange(v ?? undefined)}
        />
      )
      break
    case 'repeater':
      control = <RepeaterField field={field} value={value} onChange={onChange} disabled={disabled} />
      break
    case 'snippet':
      control = <SnippetField value={str} onChange={onChange} disabled={disabled} />
      break
    default:
      control = (
        <Input
          id={id}
          type={field.kind === 'url' ? 'url' : 'text'}
          value={str}
          placeholder={field.placeholder}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
        />
      )
  }

  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id} className="text-xs">
        {field.label}
        {field.required && <span className="ml-0.5 text-destructive">*</span>}
      </Label>
      {control}
      {field.description && <p className="text-[11px] leading-snug text-muted-foreground">{field.description}</p>}
    </div>
  )
}

// ---------- snippet picker (for the `ref` block) ----------

function SnippetField({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string | undefined) => void
  disabled: boolean
}) {
  const { data, loading } = useQuery(CmsSnippetsDocument, { fetchPolicy: 'cache-first' })
  const options = (data?.cmsSnippets ?? []).map((s) => ({ value: s.slug, label: s.name, hint: s.slug }))
  return (
    <div className={disabled ? 'pointer-events-none opacity-60' : undefined}>
      <AutocompletePicker
        options={options}
        value={value || ''}
        onChange={(v) => onChange((typeof v === 'string' ? v : v[0]) || undefined)}
        placeholder="Choose a reusable section…"
        emptyMessage="No reusable sections yet - create one under Sections."
        loading={loading}
      />
    </div>
  )
}

// ---------- repeater (array of items) ----------

function RepeaterField({
  field,
  value,
  onChange,
  disabled,
}: {
  field: CmsBlockField
  value: unknown
  onChange: (v: unknown) => void
  disabled: boolean
}) {
  const items: Array<Record<string, unknown>> = Array.isArray(value)
    ? (value as Array<Record<string, unknown>>)
    : []
  const itemFields = field.itemFields ?? []

  // Store undefined (not []) when empty so setProp deletes the prop
  const commit = (next: Array<Record<string, unknown>>) => onChange(next.length ? next : undefined)

  function setItemProp(i: number, key: string, val: unknown) {
    commit(
      items.map((it, j) => {
        if (j !== i) return it
        const c = { ...it }
        if (val === undefined || val === '') delete c[key]
        else c[key] = val
        return c
      }),
    )
  }
  function move(i: number, dir: -1 | 1) {
    const j = i + dir
    if (j < 0 || j >= items.length) return
    const next = [...items]
    ;[next[i], next[j]] = [next[j], next[i]]
    commit(next)
  }

  return (
    <div className="grid gap-2">
      {items.map((it, i) => (
        <div key={i} className="grid gap-2 rounded-md border bg-muted/30 p-2">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">Item {i + 1}</span>
            <div className="flex items-center gap-0.5">
              <Button type="button" variant="ghost" size="icon" className="size-6" disabled={disabled || i === 0} onClick={() => move(i, -1)} aria-label="Move up">
                <ChevronUpIcon className="size-3.5" />
              </Button>
              <Button type="button" variant="ghost" size="icon" className="size-6" disabled={disabled || i === items.length - 1} onClick={() => move(i, 1)} aria-label="Move down">
                <ChevronDownIcon className="size-3.5" />
              </Button>
              <Button type="button" variant="ghost" size="icon" className="size-6 text-muted-foreground hover:text-destructive" disabled={disabled} onClick={() => commit(items.filter((_, j) => j !== i))} aria-label="Remove item">
                <Trash2Icon className="size-3.5" />
              </Button>
            </div>
          </div>
          {itemFields.map((f) => (
            <FieldInput key={f.key} field={f} value={it[f.key]} onChange={(v) => setItemProp(i, f.key, v)} disabled={disabled} />
          ))}
        </div>
      ))}
      {!disabled && (
        <Button type="button" variant="outline" size="sm" className="border-dashed" onClick={() => commit([...items, {}])}>
          <PlusIcon className="size-4" />
          {field.addLabel ?? 'Add item'}
        </Button>
      )}
    </div>
  )
}

// ---------- one block card (accordion) ----------

// datetime-local (local wall-clock) ↔ ISO (what the block stores + the backend
// parses as RFC3339)
function isoToLocal(iso: unknown): string {
  if (typeof iso !== 'string' || !iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function localToIso(local: string): string | undefined {
  if (!local) return undefined
  const d = new Date(local)
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString()
}

function BlockCard({
  block,
  open,
  onToggle,
  onChange,
  onDelete,
  disabled,
}: {
  block: CmsBlock
  open: boolean
  onToggle: () => void
  onChange: (next: CmsBlock) => void
  onDelete: () => void
  disabled: boolean
}) {
  const fid = useId()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: block.id })
  const desc = blockDescriptor(block.type)
  const Icon = desc?.icon ?? GripVerticalIcon

  // Only the fields relevant to the current props (e.g. product_grid shows the
  // Collection picker only when source = collection)
  const fields = (desc?.fields ?? []).filter(
    (f) => !f.showIf || String(block.props[f.showIf.key] ?? '') === f.showIf.equals,
  )
  const mainFields = fields.filter((f) => !f.advanced)
  const advancedFields = fields.filter((f) => f.advanced)
  const [advancedOpen, setAdvancedOpen] = useState(false)

  function setProp(key: string, val: unknown) {
    const props = { ...block.props }
    if (val === undefined || val === '') delete props[key]
    else props[key] = val
    onChange({ ...block, props })
  }

  // Per-block visibility rule (props.visibility) - Schedule + segments are
  // enforced server-side; devices is a client-side CSS hint
  const vis = (block.props.visibility && typeof block.props.visibility === 'object'
    ? (block.props.visibility as Record<string, unknown>)
    : {}) as { startAt?: string; endAt?: string; segments?: string[]; devices?: string[] }
  const visSegments = Array.isArray(vis.segments) ? vis.segments : []
  const visDevices = Array.isArray(vis.devices) ? vis.devices : []
  const { enabled: featureEnabled } = useFeatures()
  const { data: segData } = useQuery(PrismSegmentsDocument, { skip: !open || !featureEnabled('signals') })
  const segments = segData?.experimentSegments ?? []

  function setVis(key: string, val: unknown) {
    const next: Record<string, unknown> = { ...vis }
    if (val === undefined || val === '' || (Array.isArray(val) && val.length === 0)) delete next[key]
    else next[key] = val
    const props = { ...block.props }
    if (Object.keys(next).length === 0) delete props.visibility
    else props.visibility = next
    onChange({ ...block, props })
  }
  function toggleVisArr(key: 'devices' | 'segments', item: string) {
    const cur = key === 'devices' ? visDevices : visSegments
    setVis(key, cur.includes(item) ? cur.filter((x) => x !== item) : [...cur, item])
  }
  // Named on the collapsed toggle, so a block with a rule set doesn't look like it has none
  const advancedSet = [
    ...advancedFields.filter((f) => block.props[f.key] != null && block.props[f.key] !== '').map((f) => f.label),
    ...(typeof block.props.viewEvent === 'string' && block.props.viewEvent ? ['view event'] : []),
    ...(Object.keys(vis).length > 0 ? ['visibility'] : []),
  ]

  return (
    <div
      ref={setNodeRef}
      data-block-card-id={block.id}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        'rounded-lg border bg-card transition-[border-color,box-shadow] duration-200 hover:border-primary/40 hover:shadow-sm',
        isDragging && 'z-10 opacity-70 shadow-lg',
        open && 'ring-1 ring-primary/40',
      )}
    >
      <div className="flex items-center gap-2 px-2 py-2">
        <button
          type="button"
          className="cursor-grab rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          aria-label="Drag to reorder"
          {...listeners}
          {...attributes}
        >
          <GripVerticalIcon className="size-4" />
        </button>
        <button type="button" className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left" onClick={onToggle}>
          <span className="grid size-7 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
            <Icon className="size-4" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium">{blockLabel(block.type)}</span>
            <span className="block truncate text-xs text-muted-foreground">{shortPreview(block) || 'Not configured'}</span>
          </span>
          <ChevronDownIcon className={cn('size-4 shrink-0 text-muted-foreground transition-transform duration-200', open && 'rotate-180')} />
        </button>
        {!disabled && (
          <button
            type="button"
            className="cursor-pointer rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            aria-label="Delete block"
            onClick={onDelete}
          >
            <Trash2Icon className="size-4" />
          </button>
        )}
      </div>
      <AnimatedPanel open={open}>
          <div className="grid gap-3 border-t p-3">
          {mainFields.length === 0 && !desc?.container ? (
            <p className="text-xs text-muted-foreground">Nothing to set up for this block.</p>
          ) : (
            mainFields.map((f) => (
              <FieldInput key={f.key} field={f} value={block.props[f.key]} onChange={(v) => setProp(f.key, v)} disabled={disabled} />
            ))
          )}
          {desc?.container && (
            <div className="grid gap-1.5">
              <Label className="text-xs">Blocks inside</Label>
              <div className="rounded-md border border-dashed bg-muted/20 p-2">
                <PageBlockCanvas
                  blocks={block.blocks ?? []}
                  onChange={(children) => onChange({ ...block, blocks: children })}
                  disabled={disabled}
                />
              </div>
            </div>
          )}
          <div className="grid border-t pt-2.5">
          <button
            type="button"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((o) => !o)}
            className="flex items-center gap-1.5 text-left text-xs text-muted-foreground hover:text-foreground"
          >
            <ChevronRightIcon className={cn('size-3.5 transition-transform duration-200', advancedOpen && 'rotate-90')} />
            <span className="font-medium">Advanced</span>
            <span>· {advancedSet.length > 0 ? `${advancedSet.join(', ')} set` : 'tracking and visibility'}</span>
          </button>
          <AnimatedPanel open={advancedOpen}>
          <div className="grid gap-3 pt-3">
          {advancedFields.map((f) => (
            <FieldInput key={f.key} field={f} value={block.props[f.key]} onChange={(v) => setProp(f.key, v)} disabled={disabled} />
          ))}
          {/* Universal: fire a custom Lens event when this block is first seen */}
          <div className="grid gap-1.5">
            <Label htmlFor={`${fid}-analytics-view-event`} className="text-xs text-muted-foreground">Analytics - view event</Label>
            <Input id={`${fid}-analytics-view-event`}
              value={typeof block.props.viewEvent === 'string' ? block.props.viewEvent : ''}
              onChange={(e) => setProp('viewEvent', e.target.value)}
              placeholder="e.g. hero_viewed"
              disabled={disabled}
              className="h-8 text-xs"
            />
            <span className="text-[11px] text-muted-foreground">
              Optional. Fires a custom event the first time this block scrolls into view - usable as a Prism A/B goal.
            </span>
          </div>

          {/* Universal: per-block visibility - schedule, audience, device */}
          <div className="grid gap-2 border-t pt-3">
            <Label className="text-xs text-muted-foreground">Visibility</Label>
            <div className="grid grid-cols-2 gap-2">
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">Show from</span>
                <Input
                  type="datetime-local"
                  value={isoToLocal(vis.startAt)}
                  onChange={(e) => setVis('startAt', localToIso(e.target.value))}
                  disabled={disabled}
                  className="h-8 text-xs"
                />
              </div>
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">Show until</span>
                <Input
                  type="datetime-local"
                  value={isoToLocal(vis.endAt)}
                  onChange={(e) => setVis('endAt', localToIso(e.target.value))}
                  disabled={disabled}
                  className="h-8 text-xs"
                />
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[11px] text-muted-foreground">Devices:</span>
              {(['mobile', 'desktop'] as const).map((d) => (
                <Button
                  key={d}
                  type="button"
                  size="sm"
                  variant={visDevices.includes(d) ? 'default' : 'outline'}
                  disabled={disabled}
                  onClick={() => toggleVisArr('devices', d)}
                  className="h-7 text-xs capitalize"
                >
                  {d}
                </Button>
              ))}
              <span className="text-[11px] text-muted-foreground">
                {visDevices.length === 0 || visDevices.length === 2 ? 'all' : `${visDevices[0]} only`}
              </span>
            </div>
            {segments.length > 0 && (
              <div className="grid gap-1">
                <span className="text-[11px] text-muted-foreground">Show only to segments (signed-in visitors)</span>
                <div className="flex flex-wrap gap-1">
                  {segments.map((s) => (
                    <button
                      key={s.slug}
                      type="button"
                      disabled={disabled}
                      onClick={() => toggleVisArr('segments', s.slug)}
                      className={cn(
                        'rounded-full border px-2 py-0.5 text-[11px] transition-colors',
                        visSegments.includes(s.slug)
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'text-muted-foreground hover:bg-accent',
                      )}
                    >
                      {s.name}
                    </button>
                  ))}
                </div>
              </div>
            )}
            <span className="text-[11px] text-muted-foreground">
              Schedule + segment apply server-side; device hides via CSS. Empty = always visible.
            </span>
          </div>
          </div>
          </AnimatedPanel>
          </div>
          </div>
      </AnimatedPanel>
    </div>
  )
}

// ---------- add-block menu ----------

// AddBlockMenu opens a right-hand sidebar to browse + search the block library Blocks are grouped by category and each shows its icon,
// name, a one-line description, and a "Container" tag - far more legible than a flat dropdown when the library has a dozen-plus block types
function AddBlockMenu({ onAdd }: { onAdd: (type: string) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')

  const q = query.trim().toLowerCase()
  const matches = CMS_BLOCK_TYPES.filter(
    (b) =>
      !q ||
      b.label.toLowerCase().includes(q) ||
      b.hint.toLowerCase().includes(q) ||
      b.type.includes(q),
  )
  const groups = BLOCK_CATEGORY_ORDER.map((cat) => ({
    cat,
    items: matches.filter((b) => blockCategory(b.type) === cat),
  })).filter((g) => g.items.length > 0)

  function pick(type: string) {
    onAdd(type)
    setOpen(false)
    setQuery('')
  }

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button type="button" variant="outline" className="w-full border-dashed">
          <PlusIcon className="size-4" />
          Add block
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-md">
        <SheetHeader className="space-y-3 border-b px-5 py-4">
          <div>
            <SheetTitle>Add a block</SheetTitle>
            <SheetDescription>Choose a block to append to the page.</SheetDescription>
          </div>
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search blocks…"
              className="h-9 pl-8"
            />
          </div>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto px-5 py-4">
          {groups.length === 0 ? (
            <p className="py-10 text-center text-sm text-muted-foreground">No blocks match “{query}”.</p>
          ) : (
            <div className="grid gap-5">
              {groups.map((g) => (
                <div key={g.cat} className="grid gap-2">
                  <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{g.cat}</p>
                  <div className="grid gap-2">
                    {g.items.map((b) => {
                      const Icon = b.icon
                      return (
                        <button
                          key={b.type}
                          type="button"
                          onClick={() => pick(b.type)}
                          className="group flex items-start gap-3 rounded-lg border bg-card px-3 py-2.5 text-left transition-colors hover:border-primary/40 hover:bg-accent"
                        >
                          <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-md bg-primary/10 text-primary">
                            <Icon className="size-5" />
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="flex items-center gap-2">
                              <span className="text-sm font-medium">{b.label}</span>
                              {b.container && (
                                <Badge variant="outline" className="h-4 px-1 text-[10px]">Container</Badge>
                              )}
                            </span>
                            <span className="mt-0.5 block text-xs leading-snug text-muted-foreground">{b.hint}</span>
                          </span>
                          <PlusIcon className="mt-1 size-4 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                        </button>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

// ---------- assembled builder ----------

export function PageBlockCanvas({
  blocks,
  onChange,
  disabled = false,
  focusBlockId = null,
  focusNonce = 0,
}: {
  blocks: CmsBlock[]
  onChange: (next: CmsBlock[]) => void
  disabled?: boolean
  // In-context selection from the live preview - When focusNonce bumps, open focusBlockId's card and scroll it into view - Only the
  // top-level canvas (wired to the preview panel) passes these; nested container canvases omit them and keep their own local open state
  focusBlockId?: string | null
  focusNonce?: number
}) {
  const [openId, setOpenId] = useState<string | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  // Admin-defined custom block types join the palette at runtime - the tick
  // forces a re-render once registration mutates the module registry
  const { data: customTypesData } = useQuery(CmsBlockTypesDocument)
  const [, setCustomTick] = useState(0)
  useEffect(() => {
    const list = customTypesData?.cmsBlockTypes
    if (list?.length && registerCustomBlockTypes(list)) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setCustomTick((t) => t + 1)
    }
  }, [customTypesData])

  // Respond to a preview click: open the matching card and bring it into view - Keyed
  // on focusNonce (not focusBlockId) so re-clicking the already-selected block
  // re-triggers - Guarded to this canvas's own blocks - a click on a block that
  // lives in another (nested) canvas is ignored here
  useEffect(() => {
    if (focusNonce === 0 || !focusBlockId) return
    if (!blocks.some((b) => b.id === focusBlockId)) return
    // Reacting to an explicit external event (a preview click, signalled via
    // focusNonce) by driving local UI - the sanctioned "subscribe to an
    // external system" use of an effect, same as the seed effect in
    // CmsPageDetail - Not a render-derived cascade
    const el = document.querySelector<HTMLElement>(`[data-block-card-id="${focusBlockId}"]`)
    if (el) holdInPlace(el, { reveal: true })
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setOpenId(focusBlockId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusNonce])

  function addBlock(type: string) {
    if (disabled) return
    const b = newBlock(type)
    onChange([...blocks, b])
    setOpenId(b.id)
  }

  function onDragEnd(e: DragEndEvent) {
    const { active, over } = e
    if (!over || disabled || active.id === over.id) return
    const from = blocks.findIndex((b) => b.id === active.id)
    const to = blocks.findIndex((b) => b.id === over.id)
    if (from === -1 || to === -1) return
    onChange(arrayMove(blocks, from, to))
  }

  // What closes over `card` when it opens: the open block's form, if that block sits above it
  function foldingAbove(id: string | null, card: HTMLElement): number {
    const open = id ? document.querySelector<HTMLElement>(`[data-block-card-id="${id}"]`) : null
    if (!open || !(open.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING)) return 0
    return open.getBoundingClientRect().height - (open.firstElementChild?.getBoundingClientRect().height ?? 0)
  }

  return (
    <div className="grid gap-2">
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={blocks.map((b) => b.id)} strategy={verticalListSortingStrategy}>
          <div className="grid gap-2">
            {blocks.length === 0 ? (
              <div className="grid place-items-center rounded-lg border border-dashed px-4 py-10 text-center text-sm text-muted-foreground">
                No blocks yet - add one below to start building the page.
              </div>
            ) : (
              blocks.map((b) => (
                <BlockCard
                  key={b.id}
                  block={b}
                  open={openId === b.id}
                  onToggle={() => {
                    const opening = openId !== b.id
                    const card = document.querySelector<HTMLElement>(`[data-block-card-id="${b.id}"]`)
                    if (opening && card) holdInPlace(card, { shrinkAbove: foldingAbove(openId, card) })
                    setOpenId(opening ? b.id : null)
                  }}
                  onChange={(next) => onChange(blocks.map((x) => (x.id === next.id ? next : x)))}
                  onDelete={() => {
                    onChange(blocks.filter((x) => x.id !== b.id))
                    setOpenId((cur) => (cur === b.id ? null : cur))
                  }}
                  disabled={disabled}
                />
              ))
            )}
          </div>
        </SortableContext>
      </DndContext>
      {!disabled && <AddBlockMenu onAdd={addBlock} />}
    </div>
  )
}
