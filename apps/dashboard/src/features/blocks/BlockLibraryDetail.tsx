import { useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
} from '@dnd-kit/core'
import { arrayMove } from '@dnd-kit/sortable'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DetailPageHeader, DetailPageLayout } from '@/components/layout/detail-page'
import { LoadingState } from '@/components/ui/loading-state'
import { cn } from '@/lib/utils'
import { useConfigScope, useUnsavedChangesGuard } from '@/lib/config-scope'
import { BlockPalette } from '@/features/email/BlockPalette'
import { EmailCanvas, END_DROP, type CanvasItem } from '@/features/email/EmailCanvas'
import { BlockInspector } from '@/features/email/BlockInspector'
import { BlockStyleControls } from '@/features/email/StyleControls'
import { RowInspector } from '@/features/email/RowInspector'
import { addToColumn, findBlock, moveChild, removeBlock, replaceBlock } from '@/features/email/treeops'
import {
  SAMPLE_VARS,
  componentMap,
  interpolateEscaped,
  newBlock,
  newBlockRef,
  type BlockComponent,
  type CustomFieldDef,
  type CustomFieldKind,
  type EmailBlock,
} from '@/features/email/blocks'
import {
  DeleteEmailBlockDocument,
  EmailBlocksDocument,
  SaveEmailBlockDocument,
} from '@/features/email/Email.gql'
import { RenderCampaignPreviewDocument } from '@/features/campaigns/Campaigns.gql'
import { toComponent } from './model'
import { usePermissions } from '@/shared/auth/usePermissions'

const MERGE_TAGS = ['store.name', 'store.logoUrl', 'storefront.url', 'customer.firstName', 'customer.email']
const FIELD_KINDS: CustomFieldKind[] = ['text', 'textarea', 'url', 'color', 'number', 'date', 'image', 'select', 'boolean']

let uid = 0
const nextId = () => `lb_${++uid}`

type Loaded = {
  channelId: string
  slug: string
  name: string
  icon: string
  fields: CustomFieldDef[]
  blocks: EmailBlock[]
  htmlTemplate: string
  textTemplate: string
  css: string
  scripted: boolean
  isNew: boolean
}

export function BlockLibraryDetail() {
  const { slug } = useParams()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()
  const isNew = !slug || slug === 'new'

  const { data, loading } = useQuery(EmailBlocksDocument, {
    variables: { channelId: effectiveChannelId ?? '' },
    skip: !effectiveChannelId,
    fetchPolicy: 'cache-and-network',
  })

  if (!isChannel || !effectiveChannelId) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Block" backTo="/emails/blocks" />
        <div className="p-6">
          <Alert>
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>Blocks are per channel - select one in the header first.</AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }

  const existing = isNew ? undefined : (data?.emailBlocks ?? []).find((b) => b.slug === slug)
  if (!isNew && !existing) {
    return (
      <DetailPageLayout>
        <DetailPageHeader title="Block" backTo="/emails/blocks" />
        <div className="p-6">{loading ? <LoadingState /> : <p className="text-sm text-muted-foreground">Block not found in this channel.</p>}</div>
      </DetailPageLayout>
    )
  }

  const initial: Loaded = isNew
    ? { channelId: effectiveChannelId, slug: '', name: '', icon: '', fields: [], blocks: [], htmlTemplate: '', textTemplate: '', css: '', scripted: false, isNew: true }
    : (() => {
        const c = toComponent(existing!)
        return {
          channelId: effectiveChannelId,
          slug: c.slug,
          name: c.name,
          icon: c.icon ?? '',
          fields: c.fields,
          blocks: c.blocks,
          htmlTemplate: c.htmlTemplate ?? '',
          textTemplate: existing!.textTemplate ?? '',
          css: c.css ?? '',
          scripted: c.scripted ?? false,
          isNew: false,
        }
      })()

  return (
    <Editor
      key={`${effectiveChannelId}::${initial.slug || 'new'}`}
      initial={initial}
      onSaved={(s) => navigate(`/emails/blocks/${s}`)}
      onDeleted={() => navigate('/emails/blocks')}
    />
  )
}

function Editor({ initial, onSaved, onDeleted }: { initial: Loaded; onSaved: (slug: string) => void; onDeleted: () => void }) {
  // Saving and deleting a block need emails.manage - a viewer gets the block to look at and preview:
  // inputs disabled, no selecting, dragging or palette, no Save or Delete
  const canEdit = usePermissions().has('emails.manage')
  const [slug, setSlug] = useState(initial.slug)
  const [name, setName] = useState(initial.name)
  const [icon, setIcon] = useState(initial.icon)
  const [fields, setFields] = useState<CustomFieldDef[]>(initial.fields)
  const [items, setItems] = useState<CanvasItem[]>(() => initial.blocks.map((b) => ({ id: nextId(), block: b })))
  const [htmlTemplate, setHtmlTemplate] = useState(initial.htmlTemplate)
  const [textTemplate, setTextTemplate] = useState(initial.textTemplate)
  const [css, setCss] = useState(initial.css)
  const [scripted, setScripted] = useState(initial.scripted)
  const [bodyMode, setBodyMode] = useState<'visual' | 'html'>(initial.htmlTemplate.trim() ? 'html' : 'visual')
  const [sel, setSel] = useState<string | null>(null)
  const [dropBefore, setDropBefore] = useState<string | null>(null)
  const [saved, setSaved] = useState(!initial.isNew)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const htmlRef = useRef<HTMLTextAreaElement | null>(null)

  const blocks = useMemo(() => items.map((i) => i.block), [items])
  const effectiveHtml = bodyMode === 'html' ? htmlTemplate : ''
  const effectiveBlocks = bodyMode === 'html' ? [] : blocks
  // CSS + scripting apply only to an HTML body
  const effectiveCss = bodyMode === 'html' ? css : ''
  const effectiveScripted = bodyMode === 'html' ? scripted : false

  const baseline = useMemo(
    () => JSON.stringify({ name: initial.name, icon: initial.icon, fields: initial.fields, blocks: initial.blocks, html: initial.htmlTemplate, text: initial.textTemplate, css: initial.css, scripted: initial.scripted }),
    [initial],
  )
  const current = JSON.stringify({ name, icon, fields, blocks: effectiveBlocks, html: effectiveHtml, text: bodyMode === 'html' ? textTemplate : '', css: effectiveCss, scripted: effectiveScripted })
  const dirty = initial.isNew ? slug.trim() !== '' || current !== baseline : current !== baseline
  useUnsavedChangesGuard(dirty, 'Unsaved block changes')

  // Sibling blocks (for the palette + ref-picker + canvas preview), scoped to
  // this channel - Exclude the block being edited from the ref options
  const { data: blocksData } = useQuery(EmailBlocksDocument, { variables: { channelId: initial.channelId } })
  const allComponents: BlockComponent[] = useMemo(() => (blocksData?.emailBlocks ?? []).map(toComponent), [blocksData])
  const components = useMemo(() => allComponents.filter((c) => c.slug !== initial.slug), [allComponents, initial.slug])
  const compMap = useMemo(() => componentMap(allComponents), [allComponents])
  const snippetList = components.map((c) => ({ slug: c.slug, name: c.name }))

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const selectedBlock = findBlock(items, sel)
  const selectedIsRow = !!selectedBlock && selectedBlock.type === 'row' && !!sel && !sel.includes('::')

  function makeBlock(type: string): EmailBlock {
    const c = compMap[type]
    return c ? newBlockRef(c) : newBlock(type)
  }
  function deleteSel(target: string) {
    setItems((prev) => removeBlock(prev, target))
    setSel((cur) => (cur === target || (cur && cur.startsWith(`${target}::`)) ? null : cur))
  }
  function updateSelected(next: EmailBlock) {
    if (sel) setItems((prev) => replaceBlock(prev, sel, next))
  }
  function onMoveChild(rowId: string, col: number, idx: number, dir: -1 | 1) {
    setItems((prev) => moveChild(prev, rowId, col, idx, dir))
  }
  function onDragOver(e: DragOverEvent) {
    const ad = e.active.data.current
    if (!ad?.fromPalette) {
      if (dropBefore !== null) setDropBefore(null)
      return
    }
    const overId = e.over ? String(e.over.id) : null
    if (!overId || overId.startsWith('col:')) {
      setDropBefore(null)
      return
    }
    setDropBefore(overId === 'canvas' ? END_DROP : overId)
  }
  function onDragEnd(e: DragEndEvent) {
    setDropBefore(null)
    const { active, over } = e
    if (!over) return
    const overId = String(over.id)
    const ad = active.data.current
    if (ad?.fromPalette) {
      const type = String(ad.blockType)
      if (overId.startsWith('col:') && type !== 'row' && type !== 'ref' && !compMap[type]) {
        const [, rowId, colStr] = overId.split(':')
        setItems((prev) => addToColumn(prev, rowId, Number(colStr), makeBlock(type)))
        return
      }
      const item = { id: nextId(), block: makeBlock(type) }
      setItems((prev) => {
        const overIdx = prev.findIndex((i) => i.id === overId)
        if (overIdx === -1) return [...prev, item]
        const next = [...prev]
        next.splice(overIdx, 0, item)
        return next
      })
      setSel(item.id)
      return
    }
    if (active.id !== over.id) {
      setItems((prev) => {
        const from = prev.findIndex((i) => i.id === active.id)
        const to = prev.findIndex((i) => i.id === overId)
        if (from === -1 || to === -1) return prev
        return arrayMove(prev, from, to)
      })
    }
  }

  // --- field schema editing ---
  function updateField(i: number, patch: Partial<CustomFieldDef>) {
    setFields((prev) => prev.map((f, j) => (j === i ? { ...f, ...patch } : f)))
  }
  function addField() {
    setFields((prev) => [...prev, { key: '', label: '', kind: 'text' }])
  }
  function removeField(i: number) {
    setFields((prev) => prev.filter((_, j) => j !== i))
  }

  function insertHtmlToken(token: string) {
    const el = htmlRef.current
    if (!el) {
      setHtmlTemplate((cur) => cur + token)
      return
    }
    const start = el.selectionStart ?? htmlTemplate.length
    const end = el.selectionEnd ?? htmlTemplate.length
    setHtmlTemplate(htmlTemplate.slice(0, start) + token + htmlTemplate.slice(end))
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(start + token.length, start + token.length)
    })
  }

  const [saveBlock] = useMutation(SaveEmailBlockDocument)
  async function onSave() {
    const cleanSlug = slug.trim()
    if (!cleanSlug || !name.trim()) {
      setError('Give the block a name and a slug.')
      return
    }
    if (bodyMode === 'html' && !htmlTemplate.trim()) {
      setError('Add an HTML template, or switch the body back to Visual.')
      return
    }
    setError(null)
    setSaving(true)
    try {
      await saveBlock({
        variables: {
          input: {
            channelId: initial.channelId,
            slug: cleanSlug,
            name: name.trim(),
            icon: icon.trim() || null,
            fields: fields.map(cleanField),
            blocks: effectiveBlocks,
            htmlTemplate: effectiveHtml,
            textTemplate: bodyMode === 'html' ? textTemplate : '',
            css: effectiveCss,
            scripted: effectiveScripted,
          },
        },
      })
      setSaved(true)
      if (initial.isNew) onSaved(cleanSlug)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  const [deleteBlock] = useMutation(DeleteEmailBlockDocument)
  async function onConfirmDelete() {
    setDeleting(true)
    try {
      await deleteBlock({ variables: { channelId: initial.channelId, slug: initial.slug } })
      onDeleted()
    } finally {
      setDeleting(false)
    }
  }

  const { data: previewData } = useQuery(RenderCampaignPreviewDocument, {
    variables: { channelId: initial.channelId, subject: name || 'Block preview', blocks, styles: {} },
    skip: !previewOpen || bodyMode === 'html',
    fetchPolicy: 'no-cache',
  })
  const htmlPreviewDoc = useMemo(() => buildHtmlPreview(htmlTemplate, fields, css), [htmlTemplate, fields, css])

  const actionsFooter = (
    <div className="space-y-2">
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex gap-2">
        {!initial.isNew && canEdit && (
          <Button type="button" variant="outline" size="sm" className="text-destructive" onClick={() => setConfirmingDelete(true)}>
            Delete
          </Button>
        )}
        <Button type="button" variant="outline" size="sm" className="flex-1" disabled={bodyMode === 'visual' && !saved} onClick={() => setPreviewOpen(true)}>
          Preview
        </Button>
        {canEdit && (
          <Button type="button" size="sm" className="flex-1" disabled={saving || !dirty} onClick={onSave}>
            {saving ? 'Saving…' : 'Save'}
          </Button>
        )}
      </div>
    </div>
  )

  // Block-level settings (identity + inputs + body mode) shown in the inspector
  // when no canvas sub-block is selected
  const blockSettings = (
    <fieldset disabled={!canEdit} className="grid gap-4">
      <div className="grid grid-cols-[1fr_auto] gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor="block-name">Block name</Label>
          <Input id="block-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Promo banner" />
        </div>
        <div className="grid w-16 gap-1.5">
          <Label htmlFor="block-icon">Icon</Label>
          <Input id="block-icon" value={icon} onChange={(e) => setIcon(e.target.value)} placeholder="✨" />
        </div>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="block-slug">Slug</Label>
        <Input
          id="block-slug"
          value={slug}
          disabled={!initial.isNew}
          onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '-'))}
          placeholder="promo-banner"
          className="font-mono"
        />
        {!initial.isNew && <p className="text-xs text-muted-foreground">The slug is the reference id and can’t be changed.</p>}
      </div>

      <div className="grid gap-1.5">
        <Label>Inputs (optional)</Label>
        <p className="text-xs text-muted-foreground">
          Add fields to make this block parameterized - authors fill them in per use; reference them as {`{{key}}`}.
        </p>
        <FieldsEditor fields={fields} onUpdate={updateField} onAdd={addField} onRemove={removeField} />
      </div>

      <div className="grid gap-1.5">
        <Label>Body</Label>
        <div className="flex gap-1 rounded-md border p-0.5">
          {(['visual', 'html'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setBodyMode(m)}
              className={cn(
                'flex-1 rounded px-2 py-1 text-xs font-medium capitalize',
                bodyMode === m ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent',
              )}
            >
              {m === 'html' ? 'HTML (advanced)' : 'Visual'}
            </button>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          {bodyMode === 'visual'
            ? 'Compose the body on the canvas. Use {{key}} in block text to slot in field values.'
            : 'Hand-author the HTML body. Field/merge values are escaped; scripts are stripped on render.'}
        </p>
      </div>
    </fieldset>
  )

  return (
    <DndContext
      sensors={canEdit ? sensors : []}
      collisionDetection={closestCenter}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
      onDragCancel={() => setDropBefore(null)}
    >
      <DetailPageLayout
        leftSidebar={bodyMode === 'visual' && canEdit ? <BlockPalette components={components} /> : undefined}
        rightSidebar={
          bodyMode === 'visual' && canEdit && selectedIsRow && selectedBlock ? (
            <aside className="flex w-80 shrink-0 flex-col border-l bg-card">
              <div className="border-b px-4 py-3">
                <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Edit Columns</h2>
              </div>
              <div className="flex-1 overflow-y-auto p-4">
                <RowInspector block={selectedBlock} onChange={updateSelected} />
              </div>
              <div className="shrink-0 border-t p-3">{actionsFooter}</div>
            </aside>
          ) : (
            <BlockInspector
              block={bodyMode === 'visual' && canEdit ? selectedBlock : null}
              onChangeBlock={updateSelected}
              subject=""
              onSubjectChange={() => {}}
              mergeTags={MERGE_TAGS}
              snippets={snippetList}
              components={compMap}
              showSubject={false}
              styleControls={bodyMode === 'visual' && canEdit && selectedBlock ? <BlockStyleControls block={selectedBlock} onChange={updateSelected} /> : null}
              emailSettings={blockSettings}
              footer={actionsFooter}
            />
          )
        }
      >
        <fieldset disabled={!canEdit} className="contents">
          {bodyMode === 'visual' ? (
            <EmailCanvas
              items={items}
              sel={canEdit ? sel : null}
              onSelect={canEdit ? setSel : () => {}}
              onDelete={canEdit ? deleteSel : () => {}}
              onMoveChild={canEdit ? onMoveChild : () => {}}
              theme={{}}
              components={compMap}
              dropBefore={dropBefore}
            />
          ) : (
            <HtmlBodyEditor
              html={htmlTemplate}
              text={textTemplate}
              css={css}
              scripted={scripted}
              onHtml={setHtmlTemplate}
              onText={setTextTemplate}
              onCss={setCss}
              onScripted={setScripted}
              onInsert={insertHtmlToken}
              fields={fields}
              htmlRef={htmlRef}
            />
          )}
        </fieldset>
      </DetailPageLayout>

      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Preview</DialogTitle>
            <DialogDescription>
              {bodyMode === 'html'
                ? 'Client-side preview with sample values (the authoritative, sanitized render is server-side).'
                : 'Rendered server-side with sample data (last saved version).'}
            </DialogDescription>
          </DialogHeader>
          <div className="overflow-hidden rounded-md border bg-white">
            <iframe
              title="Block preview"
              sandbox=""
              className="h-[440px] w-full bg-white"
              srcDoc={bodyMode === 'html' ? htmlPreviewDoc : (previewData?.renderCampaignPreview.html ?? '')}
            />
          </div>
        </DialogContent>
      </Dialog>

      <AlertDialog open={confirmingDelete} onOpenChange={(o) => !deleting && setConfirmingDelete(o)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this block?</AlertDialogTitle>
            <AlertDialogDescription>
              Emails that reference “{name}” ({initial.slug}) will render nothing in its place. This can’t be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={onConfirmDelete} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DndContext>
  )
}

// Compact field-schema builder for the inspector
function FieldsEditor({
  fields,
  onUpdate,
  onAdd,
  onRemove,
}: {
  fields: CustomFieldDef[]
  onUpdate: (i: number, patch: Partial<CustomFieldDef>) => void
  onAdd: () => void
  onRemove: (i: number) => void
}) {
  return (
    <div className="grid gap-2">
      {fields.map((f, i) => (
        <div key={i} className="grid gap-1.5 rounded-md border p-2">
          <div className="flex items-center gap-1.5">
            <Input
              value={f.key}
              onChange={(e) => onUpdate(i, { key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_') })}
              placeholder="key"
              className="h-7 font-mono text-xs"
            />
            <Select value={f.kind} onValueChange={(v) => onUpdate(i, { kind: v as CustomFieldKind })}>
              <SelectTrigger className="h-7 w-24 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FIELD_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {k}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button type="button" variant="ghost" size="icon" className="size-7 shrink-0 text-destructive" onClick={() => onRemove(i)}>
              <Trash2Icon className="size-4" />
            </Button>
          </div>
          <Input value={f.label} onChange={(e) => onUpdate(i, { label: e.target.value })} placeholder="Label" className="h-7 text-xs" />
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" onClick={onAdd}>
        <PlusIcon className="size-4" /> Add input
      </Button>
    </div>
  )
}

// HTML-body editor (main area when body mode = HTML): token chips + textarea
// for the HTML template and an optional plain-text template
function HtmlBodyEditor({
  html,
  text,
  css,
  scripted,
  onHtml,
  onText,
  onCss,
  onScripted,
  onInsert,
  fields,
  htmlRef,
}: {
  html: string
  text: string
  css: string
  scripted: boolean
  onHtml: (s: string) => void
  onText: (s: string) => void
  onCss: (s: string) => void
  onScripted: (v: boolean) => void
  onInsert: (token: string) => void
  fields: CustomFieldDef[]
  htmlRef: React.MutableRefObject<HTMLTextAreaElement | null>
}) {
  return (
    <div className="mx-auto grid max-w-3xl gap-4 p-6">
      <div className="flex items-start justify-between gap-3 rounded-md border bg-muted/20 p-3">
        <div>
          <p className="text-sm font-medium">Scripting</p>
          <p className="text-xs text-muted-foreground">
            Render the body with Liquid - conditionals, loops, and filters over the email’s data.
          </p>
        </div>
        <label className="flex shrink-0 items-center gap-2 text-xs font-medium">
          <Switch checked={scripted} onCheckedChange={onScripted} />
          {scripted ? 'On' : 'Off'}
        </label>
      </div>

      {scripted && <ScriptingHelp fields={fields} />}

      <div className="grid gap-2">
        <Label>HTML template</Label>
        <div className="flex flex-wrap gap-1">
          {fields.filter((f) => f.key).map((f) => (
            <button
              key={f.key}
              type="button"
              onClick={() => onInsert(`{{${f.key}}}`)}
              className="rounded border bg-background px-1.5 py-0.5 font-mono text-[10px] hover:bg-accent hover:text-accent-foreground"
            >
              {`{{${f.key}}}`}
            </button>
          ))}
          {MERGE_TAGS.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => onInsert(`{{${t}}}`)}
              className="rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            >
              {`{{${t}}}`}
            </button>
          ))}
        </div>
        <Textarea
          ref={htmlRef}
          value={html}
          rows={12}
          className="font-mono text-xs"
          onChange={(e) => onHtml(e.target.value)}
          placeholder={
            scripted
              ? '{% if customer.firstName %}Hi {{ customer.firstName }}!{% endif %}\n<ul>\n{% for item in order.items %}\n  <li>{{ item.name }} - {{ item.line_total }}</li>\n{% endfor %}\n</ul>'
              : '<div style="text-align:center">\n  <strong>{{label}}</strong> - {{customer.firstName}}\n</div>'
          }
        />
      </div>

      <div className="grid gap-1.5">
        <Label>Custom CSS (optional)</Label>
        <Textarea
          value={css}
          rows={4}
          className="font-mono text-xs"
          onChange={(e) => onCss(e.target.value)}
          placeholder={'.promo { color: #b91c1c; font-weight: 700 }\n@media (max-width: 600px) { .promo { font-size: 14px } }'}
        />
        <p className="text-xs text-muted-foreground">
          Injected as a &lt;style&gt; before the block. Sanitized on save/render (no @import, expression(), or script URLs).
        </p>
      </div>

      <div className="grid gap-1.5">
        <Label>Plain-text template (optional)</Label>
        <Textarea value={text} rows={3} className="font-mono text-xs" onChange={(e) => onText(e.target.value)} placeholder="{{label}} - {{customer.firstName}}" />
      </div>
    </div>
  )
}

// ScriptingHelp documents the data + Liquid syntax available to a scripted block
function ScriptingHelp({ fields }: { fields: CustomFieldDef[] }) {
  const fieldKeys = fields.filter((f) => f.key).map((f) => f.key)
  return (
    <div className="grid gap-2 rounded-md border bg-muted/20 p-3 text-xs">
      <p className="font-medium">Available data</p>
      <ul className="grid gap-0.5 text-muted-foreground">
        <li><code className="font-mono">customer</code>.firstName · lastName · email · group</li>
        <li><code className="font-mono">store</code>.name · logoUrl · <code className="font-mono">storefront.url</code></li>
        <li><code className="font-mono">order</code>.number · total · subtotal · shipping · tax · discount</li>
        <li><code className="font-mono">order.items</code> (loop) → item.name · sku · quantity · unit_price · line_total</li>
        <li><code className="font-mono">order.ship_to</code> (list of address lines)</li>
        {fieldKeys.length > 0 && (
          <li>This block’s fields: {fieldKeys.map((k) => <code key={k} className="mr-1 font-mono">{k}</code>)}</li>
        )}
      </ul>
      <p className="pt-1 font-medium">Syntax (Liquid)</p>
      <pre className="overflow-x-auto whitespace-pre-wrap rounded bg-background p-2 font-mono text-[11px] text-muted-foreground">{`{% if customer.group == "VIP" %}…{% endif %}
{% for item in order.items %}{{ item.name }} - {{ item.line_total }}{% endfor %}
{{ customer.firstName | default: "there" }}`}</pre>
      <p className="text-[11px] text-muted-foreground">Logic is sandboxed (no arbitrary code). Values render as text; the server preview / test send is authoritative.</p>
    </div>
  )
}

function cleanField(f: CustomFieldDef): CustomFieldDef {
  const out: CustomFieldDef = { key: f.key.trim(), label: f.label.trim(), kind: f.kind }
  if (f.required) out.required = true
  if (f.default) out.default = f.default
  if (f.kind === 'select' && f.options && f.options.length > 0) out.options = f.options
  return out
}

function buildHtmlPreview(html: string, fields: CustomFieldDef[], css = ''): string {
  const vars: Record<string, string> = { ...SAMPLE_VARS }
  for (const f of fields) {
    if (f.key) vars[f.key] = f.default || f.label || f.key
  }
  // Client-side approximation: substitutes {{tags}} only - Liquid logic ({% %})
  // isn't evaluated here (the server preview / test send is authoritative)
  const body = interpolateEscaped(html, vars)
  const userCss = css.trim() ? `<style>${css}</style>` : ''
  return `<!doctype html><html><head><meta charset="utf-8"><style>body{font-family:Helvetica,Arial,sans-serif;margin:12px;color:#222}</style>${userCss}</head><body>${body || '<p style="color:#999">Add an HTML template to preview.</p>'}</body></html>`
}
