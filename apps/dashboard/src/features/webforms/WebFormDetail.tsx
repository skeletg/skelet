import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  AlignLeftIcon,
  ArrowDownIcon,
  ArrowUpIcon,
  BellIcon,
  CalendarClockIcon,
  CalendarIcon,
  ChevronRightIcon,
  CircleDotIcon,
  ClockIcon,
  CopyPlusIcon,
  EyeOffIcon,
  GitBranchIcon,
  HashIcon,
  HeadingIcon,
  LinkIcon,
  ListChecksIcon,
  ListIcon,
  MailIcon,
  MinusIcon,
  PaperclipIcon,
  PhoneIcon,
  PilcrowIcon,
  PlusIcon,
  SendIcon,
  ShieldCheckIcon,
  SquareCheckIcon,
  Table2Icon,
  Trash2Icon,
  TypeIcon,
  type LucideIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import {
  CreateWebFormDocument,
  UpdateWebFormDocument,
  WebFormDocument,
  WebFormsDocument,
} from './WebForms.gql'
import { PluginSections } from '@/plugins/detail-sections'
import {
  FIELD_TYPE_LABELS,
  isChoice,
  isLayout,
  SUBFIELD_TYPES,
  type AnswerValue,
  type ConditionOp,
  type Field,
  type FieldOption,
  type FormSettings,
  type FormStatus,
  type FieldType,
  type MatrixValue,
  type RepeaterValue,
  type Rule,
} from './types'
import { isFieldVisible, visibleFields } from './visibility'

const FIELD_ICONS: Record<FieldType, LucideIcon> = {
  text: TypeIcon,
  textarea: AlignLeftIcon,
  email: MailIcon,
  number: HashIcon,
  tel: PhoneIcon,
  url: LinkIcon,
  date: CalendarIcon,
  datetime: CalendarClockIcon,
  time: ClockIcon,
  select: ListIcon,
  radio: CircleDotIcon,
  checkbox_group: ListChecksIcon,
  consent: SquareCheckIcon,
  file: PaperclipIcon,
  hidden: EyeOffIcon,
  heading: HeadingIcon,
  paragraph: PilcrowIcon,
  divider: MinusIcon,
  section: HeadingIcon,
  matrix: Table2Icon,
  repeater: CopyPlusIcon,
}

// Palette grouped by category for a scannable "add field" menu
const PALETTE_GROUPS: { label: string; types: FieldType[] }[] = [
  { label: 'Basic', types: ['text', 'textarea', 'email', 'number', 'tel', 'url'] },
  { label: 'Date & time', types: ['date', 'datetime', 'time'] },
  { label: 'Choice', types: ['select', 'radio', 'checkbox_group', 'consent'] },
  { label: 'Advanced', types: ['matrix', 'repeater', 'file', 'hidden'] },
  { label: 'Layout', types: ['section', 'heading', 'paragraph', 'divider'] },
]

const OP_LABELS: Record<ConditionOp, string> = {
  equals: 'equals',
  notEquals: 'does not equal',
  contains: 'contains',
  oneOf: 'is one of',
  empty: 'is empty',
  notEmpty: 'is not empty',
  gt: 'greater than',
  lt: 'less than',
}

let keyCounter = 0
function suggestKey(type: FieldType, existing: Set<string>): string {
  const base = type.replace(/[^a-z]/g, '') || 'field'
  let k = base
  while (existing.has(k)) {
    keyCounter += 1
    k = `${base}_${keyCounter}`
  }
  return k
}

const noKey = (t: FieldType) => isLayout(t) || t === 'section'

function blankField(type: FieldType, existing: Set<string>): Field {
  const f: Field = { type, key: noKey(type) ? '' : suggestKey(type, existing), label: FIELD_TYPE_LABELS[type] }
  if (isChoice(type)) {
    f.options = [
      { value: 'option_1', label: 'Option 1' },
      { value: 'option_2', label: 'Option 2' },
    ]
  }
  if (type === 'matrix') {
    f.rows = [{ key: 'row_1', label: 'Row 1' }, { key: 'row_2', label: 'Row 2' }]
    f.columns = [{ value: 'col_1', label: 'Column 1' }, { value: 'col_2', label: 'Column 2' }]
  }
  if (type === 'repeater') {
    f.subFields = [{ type: 'text', key: 'item_1', label: 'Field 1' }]
  }
  if (type === 'section') {
    f.label = 'Section'
  }
  return f
}

// --- builder tree helpers ---
// The form stores a FLAT field array with `section` markers; the builder edits
// it as a tree (sections own the fields that follow them) - A client-only
// __uid gives each field a stable identity for React keys + expand state that
// survives reorder; it's stripped before saving

type BuilderField = Field & { __uid: string }
type Block = { section?: BuilderField; fields: BuilderField[] }

let uidCounter = 0
function uid(): string {
  uidCounter += 1
  return `wf${uidCounter}`
}
function withUids(fields: Field[]): BuilderField[] {
  return fields.map((f) => ({ ...f, __uid: uid() }))
}
function stripUids(fields: BuilderField[]): Field[] {
  return fields.map((f) => {
    const copy: Partial<BuilderField> = { ...f }
    delete copy.__uid
    return copy as Field
  })
}
function toBlocks(fields: BuilderField[]): Block[] {
  const blocks: Block[] = [{ fields: [] }]
  for (const f of fields) {
    if (f.type === 'section') blocks.push({ section: f, fields: [] })
    else blocks[blocks.length - 1].fields.push(f)
  }
  return blocks
}
function flatten(blocks: Block[]): BuilderField[] {
  return blocks.flatMap((b) => (b.section ? [b.section, ...b.fields] : b.fields))
}
function keysOf(fields: Field[]): Set<string> {
  return new Set(fields.filter((f) => !isLayout(f.type) && f.type !== 'section').map((f) => f.key))
}

export function WebFormDetail() {
  const { id } = useParams<{ id: string }>()
  const isNew = !id || id === 'new'
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel } = useConfigScope()

  const { data, loading, error } = useQuery(WebFormDocument, {
    variables: { id: id ?? '' },
    skip: isNew,
    fetchPolicy: 'cache-and-network',
  })

  const draft = useDraft({
    name: '',
    status: 'draft' as FormStatus,
    fields: [] as BuilderField[],
    settings: {} as FormSettings,
  })
  const { name, status, fields, settings } = draft.values
  const { setName, setStatus, setFields, setSettings } = draft.setters
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [seededId, setSeededId] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [preview, setPreview] = useState<Record<string, AnswerValue>>({})

  // Seed local state from the loaded form once (React adjust-on-prop-change)
  const loaded = data?.webForm
  if (loaded && loaded.id !== seededId) {
    setSeededId(loaded.id)
    setName(loaded.name)
    setStatus(loaded.status as FormStatus)
    const seeded = withUids(Array.isArray(loaded.fields) ? (loaded.fields as Field[]) : [])
    setFields(seeded)
    setSettings((loaded.settings as FormSettings) ?? {})
    // Sections start expanded so their contents are visible; fields collapsed
    setExpanded(new Set(seeded.filter((f) => f.type === 'section').map((f) => f.__uid)))
  }

  const [createForm, { loading: creating }] = useMutation(CreateWebFormDocument, {
    refetchQueries: [{ query: WebFormsDocument, variables: { channelId: effectiveChannelId ?? '' } }],
  })
  const [updateForm, { loading: updating }] = useMutation(UpdateWebFormDocument)

  const toggle = (u: string) =>
    setExpanded((e) => {
      const n = new Set(e)
      if (n.has(u)) n.delete(u)
      else n.add(u)
      return n
    })

  function patchField(u: string, patch: Partial<Field>) {
    setFields((prev) => prev.map((f) => (f.__uid === u ? { ...f, ...patch } : f)))
  }
  function removeField(u: string) {
    setFields((prev) => prev.filter((f) => f.__uid !== u))
  }
  // Remove a section AND the fields it contains
  function removeSection(u: string) {
    setFields((prev) => flatten(toBlocks(prev).filter((b) => b.section?.__uid !== u)))
  }
  // Add a field into a section (sectionUid) or the top-level group (undefined)
  function addField(type: FieldType, sectionUid?: string) {
    const nf: BuilderField = { ...blankField(type, keysOf(fields)), __uid: uid() }
    setFields((prev) => {
      const blocks = toBlocks(prev)
      const target = sectionUid ? blocks.find((b) => b.section?.__uid === sectionUid) : blocks[0]
      ;(target ?? blocks[0]).fields.push(nf)
      return flatten(blocks)
    })
    setExpanded((e) => new Set(e).add(nf.__uid)) // open the new field for editing
  }
  function addSection() {
    const s: BuilderField = { ...blankField('section', keysOf(fields)), __uid: uid() }
    setFields((prev) => [...prev, s])
    setExpanded((e) => new Set(e).add(s.__uid))
  }
  // Move a field up/down within its own section/group
  function moveField(u: string, dir: -1 | 1) {
    setFields((prev) => {
      const blocks = toBlocks(prev)
      for (const b of blocks) {
        const i = b.fields.findIndex((f) => f.__uid === u)
        if (i < 0) continue
        const j = i + dir
        if (j < 0 || j >= b.fields.length) return prev
        ;[b.fields[i], b.fields[j]] = [b.fields[j], b.fields[i]]
        return flatten(blocks)
      }
      return prev
    })
  }
  // Move a whole section block up/down (relative to other sections)
  function moveSection(u: string, dir: -1 | 1) {
    setFields((prev) => {
      const blocks = toBlocks(prev)
      const i = blocks.findIndex((b) => b.section?.__uid === u)
      const j = i + dir
      if (i < 1 || j < 1 || j >= blocks.length) return prev // never move the top-level block[0]
      ;[blocks[i], blocks[j]] = [blocks[j], blocks[i]]
      return flatten(blocks)
    })
  }

  // Value-bearing fields before the given uid (for the condition editor)
  function priorFieldsFor(u: string): Field[] {
    const idx = fields.findIndex((f) => f.__uid === u)
    return idx <= 0 ? [] : fields.slice(0, idx)
  }

  async function save() {
    setSaveError(null)
    const input = { name: name.trim(), status, fields: stripUids(fields), settings }
    try {
      if (isNew) {
        if (!effectiveChannelId) throw new Error('Pick a channel first.')
        const res = await createForm({ variables: { channelId: effectiveChannelId, input: { ...input, slug: name.trim() } } })
        const newId = res.data?.createWebForm?.id
        if (newId) await duringCommit(() => navigate(`/cms/forms/${newId}`, { replace: true }))
      } else {
        await updateForm({ variables: { id: id!, input } })
        draft.saved()
      }
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  if (!isNew && loading && !loaded) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/cms/forms" backLabel="Web forms" title="Form" />
        <div className="p-6 text-sm text-muted-foreground">Loading…</div>
      </DetailPageLayout>
    )
  }
  if (error) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/cms/forms" backLabel="Web forms" title="Form" />
        <div className="p-6">
          <Alert variant="destructive">
            <AlertTitle>Couldn't load form</AlertTitle>
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }
  if (isNew && !isChannel) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/cms/forms" backLabel="Web forms" title="New web form" />
        <div className="p-6">
          <Alert>
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>Forms are per channel - select one in the header scope picker.</AlertDescription>
          </Alert>
        </div>
      </DetailPageLayout>
    )
  }

  const saving = creating || updating
  const blocks = toBlocks(fields)
  const inputCount = fields.filter((f) => !isLayout(f.type) && f.type !== 'section').length

  const previewPane = (
    <FormPreview
      name={name}
      fields={fields}
      answers={preview}
      settings={settings}
      onChange={(key, val) => setPreview((p) => ({ ...p, [key]: val }))}
    />
  )

  return (
    <DetailPageLayout
      // Split view: the builder scrolls on the left, the live preview owns a pinned pane on the right - Below xl the
      // pane collapses into a regular body section so the preview stays reachable on narrow screens
      rightSidebar={
        <aside className="hidden h-full w-[26rem] shrink-0 flex-col border-l bg-muted/10 xl:flex 2xl:w-[30rem]">
          <div className="border-b bg-background px-4 py-3">
            <div className="text-sm font-semibold">Live preview</div>
            <div className="text-xs text-muted-foreground">Reflects your conditional rules as you type.</div>
          </div>
          <div className="scrollbar-accent flex-1 overflow-y-auto p-4">{previewPane}</div>
        </aside>
      }
    >
      <DetailPageHeader
        backTo="/cms/forms"
        backLabel="Web forms"
        title={isNew ? 'New web form' : name || 'Form'}
        actions={
          <div className="flex items-center gap-2">
            {!isNew && (
              <Button asChild variant="outline" size="sm">
                <Link to={`/cms/forms/${id}/submissions`}>Submissions</Link>
              </Button>
            )}
            <Button size="sm" onClick={save} disabled={saving || !name.trim()}>
              {saving ? 'Saving…' : 'Save'}
            </Button>
          </div>
        }
      />
      <DetailPageBody>
        {saveError && (
          <DetailPageSection>
            <Alert variant="destructive">
              <AlertTitle>Save failed</AlertTitle>
              <AlertDescription>{saveError}</AlertDescription>
            </Alert>
          </DetailPageSection>
        )}

        <DetailPageSection title="Basics">
          <div className="grid gap-4 sm:grid-cols-[1fr_200px]">
            <div className="grid gap-1.5">
              <Label htmlFor="wf-name">Name</Label>
              <Input id="wf-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Contact form" />
            </div>
            <div className="grid gap-1.5">
              <Label>Status</Label>
              <Select value={status} onValueChange={(v) => setStatus(v as FormStatus)}>
                <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="draft">Draft</SelectItem>
                  <SelectItem value="active">Active</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection title="Fields" description={`${inputCount} input field(s)`}>
          <div className="grid gap-3">
            {fields.length === 0 && (
              <div className="rounded-lg border border-dashed p-8 text-center">
                <p className="text-sm font-medium">No fields yet</p>
                <p className="mt-1 text-xs text-muted-foreground">Add a field or a section to start building the form.</p>
              </div>
            )}

            {blocks.map((block, bi) =>
              block.section ? (
                <SectionPanel
                  key={block.section.__uid}
                  block={block}
                  open={expanded.has(block.section.__uid)}
                  expanded={expanded}
                  priorForSection={priorFieldsFor(block.section.__uid)}
                  priorFieldsFor={priorFieldsFor}
                  canUp={bi > 1}
                  canDown={bi < blocks.length - 1}
                  onToggle={toggle}
                  onPatch={patchField}
                  onAddField={(t) => addField(t, block.section!.__uid)}
                  onRemoveField={removeField}
                  onMoveField={moveField}
                  onRemoveSection={() => removeSection(block.section!.__uid)}
                  onMoveSection={(d) => moveSection(block.section!.__uid, d)}
                />
              ) : block.fields.length > 0 ? (
                <div key="top-level" className="grid gap-2">
                  {block.fields.map((f, i) => (
                    <FieldCard
                      key={f.__uid}
                      field={f}
                      open={expanded.has(f.__uid)}
                      priorFields={priorFieldsFor(f.__uid)}
                      canUp={i > 0}
                      canDown={i < block.fields.length - 1}
                      onToggle={toggle}
                      onChange={(patch) => patchField(f.__uid, patch)}
                      onMove={(d) => moveField(f.__uid, d)}
                      onRemove={() => removeField(f.__uid)}
                    />
                  ))}
                </div>
              ) : null,
            )}

            <div className="flex flex-wrap gap-2 pt-1">
              <AddFieldMenu onAdd={(t) => addField(t)} label="Add field" />
              <Button variant="outline" size="sm" onClick={addSection}>
                <HeadingIcon className="size-4" /> Add section
              </Button>
            </div>
          </div>
        </DetailPageSection>

        <DetailPageSection title="Settings">
          <SettingsEditor settings={settings} fields={fields} onChange={setSettings} />
        </DetailPageSection>

        <DetailPageSection
          title="Preview"
          description="A live preview reflecting your conditional rules."
          className="xl:hidden"
        >
          {previewPane}
        </DetailPageSection>
        <PluginSections target="web-form" entity={loaded ?? null} mode={isNew ? 'create' : 'edit'} />
      </DetailPageBody>
    </DetailPageLayout>
  )
}

// --- builder tree UI ---

// AddFieldMenu is the grouped palette dropdown (sections excluded - they're
// added via the dedicated "Add section" control)
function AddFieldMenu({
  onAdd,
  label = 'Add field',
  className,
}: {
  onAdd: (t: FieldType) => void
  label?: string
  className?: string
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" className={className}>
          <PlusIcon className="size-4" /> {label}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-96 w-56 overflow-y-auto">
        {PALETTE_GROUPS.map((g) => {
          const types = g.types.filter((t) => t !== 'section')
          if (types.length === 0) return null
          return (
            <div key={g.label}>
              <div className="px-2 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{g.label}</div>
              {types.map((t) => {
                const Icon = FIELD_ICONS[t]
                return (
                  <DropdownMenuItem key={t} onSelect={() => onAdd(t)} className="gap-2">
                    <Icon className="size-4 text-muted-foreground" />
                    {FIELD_TYPE_LABELS[t]}
                  </DropdownMenuItem>
                )
              })}
            </div>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// RowActions renders the move-up/down + delete controls used on both field and
// section headers. stopPropagation so clicks don't toggle the accordion
function RowActions({
  canUp,
  canDown,
  onMove,
  onRemove,
  removeTitle,
}: {
  canUp: boolean
  canDown: boolean
  onMove: (dir: -1 | 1) => void
  onRemove: () => void
  removeTitle: string
}) {
  return (
    <div className="flex items-center gap-0.5" onClick={(e) => e.stopPropagation()}>
      <Button variant="ghost" size="icon" className="size-7" disabled={!canUp} onClick={() => onMove(-1)} title="Move up">
        <ArrowUpIcon className="size-4" />
      </Button>
      <Button variant="ghost" size="icon" className="size-7" disabled={!canDown} onClick={() => onMove(1)} title="Move down">
        <ArrowDownIcon className="size-4" />
      </Button>
      <Button variant="ghost" size="icon" className="size-7 text-muted-foreground hover:text-destructive" onClick={onRemove} title={removeTitle}>
        <Trash2Icon className="size-4" />
      </Button>
    </div>
  )
}

function isConditional(f: Field): boolean {
  return !!f.visibleIf && ((f.visibleIf.rules?.length ?? 0) > 0 || (f.visibleIf.groups?.length ?? 0) > 0)
}

// FieldCard is one input rendered as a collapsible accordion item: a summary
// row that expands to the full editor
function FieldCard({
  field,
  open,
  priorFields,
  canUp,
  canDown,
  onToggle,
  onChange,
  onMove,
  onRemove,
}: {
  field: BuilderField
  open: boolean
  priorFields: Field[]
  canUp: boolean
  canDown: boolean
  onToggle: (uid: string) => void
  onChange: (patch: Partial<Field>) => void
  onMove: (dir: -1 | 1) => void
  onRemove: () => void
}) {
  const Icon = FIELD_ICONS[field.type]
  return (
    <div className={`rounded-lg border bg-card ${open ? 'ring-1 ring-primary/20' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        onClick={() => onToggle(field.__uid)}
        onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onToggle(field.__uid)}
        className="flex cursor-pointer items-center gap-2.5 px-2.5 py-2 text-sm"
      >
        <ChevronRightIcon className={`size-4 shrink-0 text-muted-foreground transition-transform ${open ? 'rotate-90' : ''}`} />
        <span className={`flex size-6 shrink-0 items-center justify-center rounded-md ${open ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'}`}>
          <Icon className="size-3.5" />
        </span>
        <span className="min-w-0 flex-1 truncate">
          {field.label || <span className="text-muted-foreground">Untitled</span>}
          {field.required && <span className="ml-1 text-destructive">*</span>}
        </span>
        {isConditional(field) && (
          <span title="Conditional field">
            <GitBranchIcon className="size-3.5 shrink-0 text-muted-foreground" />
          </span>
        )}
        <span className="shrink-0 text-[10px] uppercase tracking-wide text-muted-foreground/70">
          {field.type === 'checkbox_group' ? 'checkbox' : field.type}
        </span>
        <RowActions canUp={canUp} canDown={canDown} onMove={onMove} onRemove={onRemove} removeTitle="Delete field" />
      </div>
      {open && (
        <div className="border-t p-4">
          <FieldEditorBody field={field} priorFields={priorFields} onChange={onChange} />
        </div>
      )}
    </div>
  )
}

// SectionPanel is a collapsible group: its settings ("section data") plus the
// fields it contains, each an expandable FieldCard
function SectionPanel({
  block,
  open,
  expanded,
  priorForSection,
  priorFieldsFor,
  canUp,
  canDown,
  onToggle,
  onPatch,
  onAddField,
  onRemoveField,
  onMoveField,
  onRemoveSection,
  onMoveSection,
}: {
  block: Block
  open: boolean
  expanded: Set<string>
  priorForSection: Field[]
  priorFieldsFor: (uid: string) => Field[]
  canUp: boolean
  canDown: boolean
  onToggle: (uid: string) => void
  onPatch: (uid: string, patch: Partial<Field>) => void
  onAddField: (t: FieldType) => void
  onRemoveField: (uid: string) => void
  onMoveField: (uid: string, dir: -1 | 1) => void
  onRemoveSection: () => void
  onMoveSection: (dir: -1 | 1) => void
}) {
  const sec = block.section!
  return (
    <div className={`overflow-hidden rounded-xl border bg-muted/20 ${open ? 'ring-1 ring-primary/20' : ''}`}>
      <div
        role="button"
        tabIndex={0}
        onClick={() => onToggle(sec.__uid)}
        onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onToggle(sec.__uid)}
        className="flex cursor-pointer items-center gap-2.5 px-3 py-2.5"
      >
        <ChevronRightIcon className={`size-4 shrink-0 text-muted-foreground transition-transform ${open ? 'rotate-90' : ''}`} />
        <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-foreground/5 text-foreground">
          <HeadingIcon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold">{sec.label || 'Section'}</div>
          <div className="text-[11px] text-muted-foreground">
            Section · {block.fields.length} field{block.fields.length === 1 ? '' : 's'}
            {isConditional(sec) && ' · conditional'}
          </div>
        </div>
        <RowActions canUp={canUp} canDown={canDown} onMove={onMoveSection} onRemove={onRemoveSection} removeTitle="Delete section & its fields" />
      </div>
      {open && (
        <div className="grid gap-4 border-t bg-background p-4">
          <SectionSettings section={sec} priorFields={priorForSection} onChange={(patch) => onPatch(sec.__uid, patch)} />
          <div className="grid gap-2">
            <div className="text-xs font-medium text-muted-foreground">Fields in this section</div>
            {block.fields.length === 0 && (
              <p className="rounded-md border border-dashed p-4 text-center text-xs text-muted-foreground">
                No fields in this section yet.
              </p>
            )}
            {block.fields.map((f, i) => (
              <FieldCard
                key={f.__uid}
                field={f}
                open={expanded.has(f.__uid)}
                priorFields={priorFieldsFor(f.__uid)}
                canUp={i > 0}
                canDown={i < block.fields.length - 1}
                onToggle={onToggle}
                onChange={(patch) => onPatch(f.__uid, patch)}
                onMove={(d) => onMoveField(f.__uid, d)}
                onRemove={() => onRemoveField(f.__uid)}
              />
            ))}
          </div>
          <AddFieldMenu onAdd={onAddField} label="Add field to section" className="w-fit border-dashed" />
        </div>
      )}
    </div>
  )
}

// SectionSettings is the "section data" editor: title, description, style, and
// the section-level conditional visibility
function SectionSettings({
  section,
  priorFields,
  onChange,
}: {
  section: Field
  priorFields: Field[]
  onChange: (patch: Partial<Field>) => void
}) {
  return (
    <div className="grid gap-3 rounded-lg border bg-muted/30 p-3">
      <div className="grid gap-1.5 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label>Section title</Label>
          <Input value={section.label} onChange={(e) => onChange({ label: e.target.value })} />
        </div>
        <div className="grid gap-1.5">
          <Label>Description</Label>
          <Input value={section.helpText ?? ''} onChange={(e) => onChange({ helpText: e.target.value })} />
        </div>
      </div>
      <label className="flex items-center gap-2 text-sm">
        <Switch checked={!!section.boxed} onCheckedChange={(v) => onChange({ boxed: v })} />
        Boxed style (bordered group on the storefront)
      </label>
      <ConditionEditor field={section} priorFields={priorFields} onChange={onChange} />
    </div>
  )
}

// FieldEditorBody is the field's editor without a header row (the FieldCard supplies the header) - Sections use
// SectionSettings, so this handles only scalar/choice/composite/layout fields
function FieldEditorBody({
  field,
  priorFields,
  onChange,
}: {
  field: Field
  priorFields: Field[]
  onChange: (patch: Partial<Field>) => void
}) {
  const layout = isLayout(field.type)
  const matrix = field.type === 'matrix'
  const repeater = field.type === 'repeater'
  const composite = matrix || repeater
  const scalar = !layout && !composite
  return (
    <div className="grid gap-4">
      <div className="grid gap-1.5">
        <Label>Label</Label>
        <Input value={field.label} onChange={(e) => onChange({ label: e.target.value })} />
      </div>

      {(scalar || composite) && (
        <div className="grid gap-1.5 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label>Field key</Label>
            <Input
              value={field.key}
              onChange={(e) => onChange({ key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_') })}
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">Used in submissions + conditions. Lowercase.</p>
          </div>
          {scalar && (
            <div className="grid gap-1.5">
              <Label>Placeholder</Label>
              <Input value={field.placeholder ?? ''} onChange={(e) => onChange({ placeholder: e.target.value })} />
            </div>
          )}
        </div>
      )}

      {!layout && (
        <div className="grid gap-1.5">
          <Label>Help text</Label>
          <Input value={field.helpText ?? ''} onChange={(e) => onChange({ helpText: e.target.value })} />
        </div>
      )}

      {(scalar || composite) && (
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={!!field.required} onCheckedChange={(v) => onChange({ required: v })} />
          Required
        </label>
      )}

      {isChoice(field.type) && (
        <div className="grid gap-1.5">
          <Label>Options</Label>
          <OptionListEditor options={field.options ?? []} onChange={(o) => onChange({ options: o })} />
        </div>
      )}

      {scalar && field.type !== 'file' && field.type !== 'consent' && (
        <ValidationEditor field={field} onChange={onChange} />
      )}

      {field.type === 'file' && (
        <div className="grid gap-1.5 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label>Max file size (MB)</Label>
            <Input
              type="number"
              value={field.maxFileBytes ? Math.round(field.maxFileBytes / (1024 * 1024)) : ''}
              onChange={(e) =>
                onChange({ maxFileBytes: e.target.value ? Number(e.target.value) * 1024 * 1024 : undefined })
              }
            />
          </div>
          <div className="grid gap-1.5">
            <Label>Accepted types</Label>
            <Input
              value={(field.acceptMimes ?? []).join(', ')}
              placeholder="image/*, application/pdf"
              onChange={(e) =>
                onChange({ acceptMimes: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })
              }
            />
          </div>
        </div>
      )}

      {matrix && <MatrixEditor field={field} onChange={onChange} />}
      {repeater && <RepeaterEditor field={field} onChange={onChange} />}

      {!layout && <ConditionEditor field={field} priorFields={priorFields} onChange={onChange} />}
    </div>
  )
}

// OptionListEditor edits a list of {value,label} - reused for choice options
// and matrix columns
function OptionListEditor({
  options,
  onChange,
  addLabel = 'Add option',
}: {
  options: FieldOption[]
  onChange: (options: FieldOption[]) => void
  addLabel?: string
}) {
  return (
    <div className="grid gap-1.5">
      <div className="grid gap-1.5">
        {options.map((o, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input
              className="flex-1 text-sm"
              value={o.label}
              placeholder="Label"
              onChange={(e) => {
                const next = [...options]
                const label = e.target.value
                next[i] = { label, value: o.value || label.toLowerCase().replace(/[^a-z0-9]+/g, '_') }
                onChange(next)
              }}
            />
            <Input
              className="w-32 font-mono text-xs"
              value={o.value}
              placeholder="value"
              onChange={(e) => {
                const next = [...options]
                next[i] = { ...o, value: e.target.value }
                onChange(next)
              }}
            />
            <Button variant="ghost" size="icon" className="size-7 text-destructive" onClick={() => onChange(options.filter((_, j) => j !== i))}>
              <Trash2Icon className="size-4" />
            </Button>
          </div>
        ))}
      </div>
      <Button
        variant="outline"
        size="sm"
        className="w-fit"
        onClick={() => onChange([...options, { value: `option_${options.length + 1}`, label: `Option ${options.length + 1}` }])}
      >
        <PlusIcon className="size-4" /> {addLabel}
      </Button>
    </div>
  )
}

// MatrixEditor configures a grid: rows (questions) + columns (choices) + multi
function MatrixEditor({ field, onChange }: { field: Field; onChange: (patch: Partial<Field>) => void }) {
  const rows = field.rows ?? []
  return (
    <div className="grid gap-3 rounded-md border border-dashed p-3">
      <div className="grid gap-1.5">
        <Label>Rows (one question per row)</Label>
        <div className="grid gap-1.5">
          {rows.map((r, i) => (
            <div key={i} className="flex items-center gap-2">
              <Input
                className="flex-1 text-sm"
                value={r.label}
                placeholder="Row label"
                onChange={(e) => {
                  const next = [...rows]
                  const label = e.target.value
                  next[i] = { label, key: r.key || label.toLowerCase().replace(/[^a-z0-9]+/g, '_') }
                  onChange({ rows: next })
                }}
              />
              <Input
                className="w-28 font-mono text-xs"
                value={r.key}
                placeholder="key"
                onChange={(e) => {
                  const next = [...rows]
                  next[i] = { ...r, key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_') }
                  onChange({ rows: next })
                }}
              />
              <Button variant="ghost" size="icon" className="size-7 text-destructive" onClick={() => onChange({ rows: rows.filter((_, j) => j !== i) })}>
                <Trash2Icon className="size-4" />
              </Button>
            </div>
          ))}
        </div>
        <Button variant="outline" size="sm" className="w-fit" onClick={() => onChange({ rows: [...rows, { key: `row_${rows.length + 1}`, label: `Row ${rows.length + 1}` }] })}>
          <PlusIcon className="size-4" /> Add row
        </Button>
      </div>
      <div className="grid gap-1.5">
        <Label>Columns (the choices)</Label>
        <OptionListEditor options={field.columns ?? []} onChange={(c) => onChange({ columns: c })} addLabel="Add column" />
      </div>
      <label className="flex items-center gap-2 text-sm">
        <Switch checked={!!field.multi} onCheckedChange={(v) => onChange({ multi: v })} />
        Allow multiple selections per row
      </label>
    </div>
  )
}

// RepeaterEditor configures the per-instance sub-fields + instance-count bounds
function RepeaterEditor({ field, onChange }: { field: Field; onChange: (patch: Partial<Field>) => void }) {
  const subs = field.subFields ?? []
  function patchSub(i: number, patch: Partial<Field>) {
    onChange({ subFields: subs.map((s, j) => (j === i ? { ...s, ...patch } : s)) })
  }
  return (
    <div className="grid gap-3 rounded-md border border-dashed p-3">
      <Label>Repeatable fields (per entry)</Label>
      <div className="grid gap-2">
        {subs.map((s, i) => (
          <div key={i} className="grid gap-2 rounded-md border bg-background p-2">
            <div className="flex items-center gap-2">
              <Select value={s.type} onValueChange={(v) => patchSub(i, { type: v as FieldType })}>
                <SelectTrigger className="h-8 w-40 text-xs"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {SUBFIELD_TYPES.map((t) => <SelectItem key={t} value={t}>{FIELD_TYPE_LABELS[t]}</SelectItem>)}
                </SelectContent>
              </Select>
              <Input className="h-8 flex-1 text-sm" value={s.label} placeholder="Label" onChange={(e) => patchSub(i, { label: e.target.value })} />
              <Input className="h-8 w-24 font-mono text-xs" value={s.key} placeholder="key" onChange={(e) => patchSub(i, { key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_') })} />
              <label className="flex items-center gap-1 text-xs">
                <Switch checked={!!s.required} onCheckedChange={(v) => patchSub(i, { required: v })} /> Req
              </label>
              <Button variant="ghost" size="icon" className="size-7 text-destructive" onClick={() => onChange({ subFields: subs.filter((_, j) => j !== i) })}>
                <Trash2Icon className="size-4" />
              </Button>
            </div>
            {isChoice(s.type) && (
              <OptionListEditor options={s.options ?? []} onChange={(o) => patchSub(i, { options: o })} />
            )}
          </div>
        ))}
      </div>
      <Button
        variant="outline"
        size="sm"
        className="w-fit"
        onClick={() => onChange({ subFields: [...subs, { type: 'text', key: `item_${subs.length + 1}`, label: `Field ${subs.length + 1}` }] })}
      >
        <PlusIcon className="size-4" /> Add field
      </Button>
      <div className="grid grid-cols-2 gap-2">
        <div className="grid gap-1">
          <Label className="text-xs">Min entries</Label>
          <Input type="number" value={field.minItems ?? ''} onChange={(e) => onChange({ minItems: e.target.value ? Number(e.target.value) : undefined })} />
        </div>
        <div className="grid gap-1">
          <Label className="text-xs">Max entries</Label>
          <Input type="number" value={field.maxItems ?? ''} onChange={(e) => onChange({ maxItems: e.target.value ? Number(e.target.value) : undefined })} />
        </div>
      </div>
    </div>
  )
}

function ValidationEditor({ field, onChange }: { field: Field; onChange: (patch: Partial<Field>) => void }) {
  const v = field.validation ?? {}
  const num = (x: number | undefined) => (x === undefined ? '' : String(x))
  const patchV = (patch: Partial<typeof v>) => onChange({ validation: { ...v, ...patch } })
  const isNumber = field.type === 'number'
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs text-muted-foreground">Validation (optional)</Label>
      <div className="grid grid-cols-2 gap-2">
        {isNumber ? (
          <>
            <Input type="number" placeholder="Min value" value={num(v.min)} onChange={(e) => patchV({ min: e.target.value ? Number(e.target.value) : undefined })} />
            <Input type="number" placeholder="Max value" value={num(v.max)} onChange={(e) => patchV({ max: e.target.value ? Number(e.target.value) : undefined })} />
          </>
        ) : (
          <>
            <Input type="number" placeholder="Min length" value={num(v.minLength)} onChange={(e) => patchV({ minLength: e.target.value ? Number(e.target.value) : undefined })} />
            <Input type="number" placeholder="Max length" value={num(v.maxLength)} onChange={(e) => patchV({ maxLength: e.target.value ? Number(e.target.value) : undefined })} />
          </>
        )}
      </div>
      <Input placeholder="Pattern (regex, optional)" className="font-mono text-xs" value={v.pattern ?? ''} onChange={(e) => patchV({ pattern: e.target.value })} />
    </div>
  )
}

function ConditionEditor({
  field,
  priorFields,
  onChange,
}: {
  field: Field
  priorFields: Field[]
  onChange: (patch: Partial<Field>) => void
}) {
  const candidates = priorFields.filter((f) => !isLayout(f.type))
  const group = field.visibleIf
  const rules = group?.rules ?? []
  const enabled = !!group

  function setRules(next: Rule[]) {
    onChange({ visibleIf: { logic: group?.logic ?? 'and', rules: next } })
  }

  if (candidates.length === 0) {
    return <p className="text-[11px] text-muted-foreground">Add fields above this one to make it conditional on their answers.</p>
  }

  return (
    <div className="grid gap-2 rounded-md border border-dashed p-3">
      <label className="flex items-center gap-2 text-sm">
        <Checkbox
          checked={enabled}
          onCheckedChange={(v) => onChange({ visibleIf: v ? { logic: 'and', rules: [{ field: candidates[0].key, op: 'equals', value: '' }] } : undefined })}
        />
        Only show this field when…
      </label>
      {enabled && (
        <div className="grid gap-2">
          {rules.length > 1 && (
            <Select value={group?.logic ?? 'and'} onValueChange={(v) => onChange({ visibleIf: { logic: v as 'and' | 'or', rules } })}>
              <SelectTrigger className="h-8 w-40 text-xs"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="and">All rules match (AND)</SelectItem>
                <SelectItem value="or">Any rule matches (OR)</SelectItem>
              </SelectContent>
            </Select>
          )}
          {rules.map((r, i) => (
            <div key={i} className="flex flex-wrap items-center gap-1.5">
              <Select value={r.field} onValueChange={(v) => setRules(rules.map((x, j) => (j === i ? { ...x, field: v } : x)))}>
                <SelectTrigger className="h-8 w-36 text-xs"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {candidates.map((c) => <SelectItem key={c.key} value={c.key}>{c.label || c.key}</SelectItem>)}
                </SelectContent>
              </Select>
              <Select value={r.op} onValueChange={(v) => setRules(rules.map((x, j) => (j === i ? { ...x, op: v as ConditionOp } : x)))}>
                <SelectTrigger className="h-8 w-36 text-xs"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {(Object.keys(OP_LABELS) as ConditionOp[]).map((op) => <SelectItem key={op} value={op}>{OP_LABELS[op]}</SelectItem>)}
                </SelectContent>
              </Select>
              {r.op !== 'empty' && r.op !== 'notEmpty' && (
                <Input
                  className="h-8 w-32 text-xs"
                  value={r.value ?? ''}
                  placeholder="value"
                  onChange={(e) => setRules(rules.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))}
                />
              )}
              <Button variant="ghost" size="icon" className="size-7 text-destructive" onClick={() => setRules(rules.filter((_, j) => j !== i))}>
                <Trash2Icon className="size-4" />
              </Button>
            </div>
          ))}
          <Button variant="outline" size="sm" className="w-fit" onClick={() => setRules([...rules, { field: candidates[0].key, op: 'equals', value: '' }])}>
            <PlusIcon className="size-4" /> Add rule
          </Button>
        </div>
      )}
    </div>
  )
}

function SettingsEditor({
  settings,
  fields,
  onChange,
}: {
  settings: FormSettings
  fields: Field[]
  onChange: (s: FormSettings) => void
}) {
  const patch = (p: Partial<FormSettings>) => onChange({ ...settings, ...p })
  const emailFields = fields.filter((f) => f.type === 'email')
  const conf = settings.confirmation ?? {}
  return (
    <div className="grid gap-4 lg:grid-cols-2 lg:items-start">
      <SettingsCard icon={SendIcon} title="After submit" description="What the visitor sees when they submit.">
        <div className="grid gap-1.5">
          <Label>Submit button label</Label>
          <Input value={settings.submitLabel ?? ''} placeholder="Submit" onChange={(e) => patch({ submitLabel: e.target.value })} />
        </div>
        <div className="grid gap-1.5">
          <Label>Success message</Label>
          <Textarea value={settings.successMessage ?? ''} placeholder="Thanks - we'll be in touch." onChange={(e) => patch({ successMessage: e.target.value })} />
        </div>
        <div className="grid gap-1.5">
          <Label>Redirect URL (optional)</Label>
          <Input value={settings.redirectUrl ?? ''} placeholder="/thank-you" onChange={(e) => patch({ redirectUrl: e.target.value })} />
          <p className="text-[11px] text-muted-foreground">If set, the visitor is redirected here instead of seeing the message.</p>
        </div>
      </SettingsCard>

      <SettingsCard icon={BellIcon} title="Notifications" description="Who gets told about a submission.">
        <div className="grid gap-1.5">
          <Label>Notify these emails (comma-separated)</Label>
          <Input
            value={(settings.notifyEmails ?? []).join(', ')}
            placeholder="team@example.com"
            onChange={(e) => patch({ notifyEmails: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })}
          />
        </div>
        <div className="grid gap-2 rounded-lg border bg-muted/30 p-3">
          <label className="flex items-center gap-2 text-sm font-medium">
            <Switch checked={!!conf.enabled} onCheckedChange={(v) => patch({ confirmation: { ...conf, enabled: v } })} />
            Confirmation email to the submitter
          </label>
          {conf.enabled && (
            <div className="grid gap-2 pt-1">
              <div className="grid gap-1.5">
                <Label className="text-xs">Send to which email field</Label>
                <Select value={conf.toFieldKey ?? ''} onValueChange={(v) => patch({ confirmation: { ...conf, toFieldKey: v } })}>
                  <SelectTrigger className="w-full h-8 text-xs"><SelectValue placeholder="Select an email field" /></SelectTrigger>
                  <SelectContent>
                    {emailFields.length === 0 && <div className="px-2 py-1.5 text-xs text-muted-foreground">Add an email field first</div>}
                    {emailFields.map((f) => <SelectItem key={f.key} value={f.key}>{f.label || f.key}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <Input placeholder="Subject" value={conf.subject ?? ''} onChange={(e) => patch({ confirmation: { ...conf, subject: e.target.value } })} />
              <Textarea placeholder="Message body" value={conf.body ?? ''} onChange={(e) => patch({ confirmation: { ...conf, body: e.target.value } })} />
            </div>
          )}
        </div>
      </SettingsCard>

      <SettingsCard icon={ShieldCheckIcon} title="Spam & integrations" description="Protection and downstream automation." className="lg:col-span-2">
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="flex items-center gap-2 rounded-lg border p-3 text-sm">
            <Switch checked={!!settings.requireCaptcha} onCheckedChange={(v) => patch({ requireCaptcha: v })} />
            <span>
              Require CAPTCHA
              <span className="block text-xs font-normal text-muted-foreground">On top of the built-in honeypot + rate limiting.</span>
            </span>
          </label>
          <label className="flex items-center gap-2 rounded-lg border p-3 text-sm">
            <Switch checked={!!settings.flowTrigger} onCheckedChange={(v) => patch({ flowTrigger: v })} />
            <span>
              Fire the form-submitted flow trigger
              <span className="block text-xs font-normal text-muted-foreground">Kick off a Pulse marketing flow.</span>
            </span>
          </label>
        </div>
        <div className="grid gap-1.5 sm:max-w-xs">
          <Label>Record consent under (optional)</Label>
          <Input value={settings.consentKind ?? ''} placeholder="marketing_email" onChange={(e) => patch({ consentKind: e.target.value })} />
        </div>
      </SettingsCard>
    </div>
  )
}

function SettingsCard({
  icon: Icon,
  title,
  description,
  className,
  children,
}: {
  icon: LucideIcon
  title: string
  description: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={`grid content-start gap-3 rounded-xl border bg-card p-5 shadow-sm ${className ?? ''}`}>
      <div className="flex items-center gap-2.5">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <Icon className="size-4" />
        </span>
        <div>
          <div className="text-sm font-semibold leading-tight">{title}</div>
          <div className="text-xs text-muted-foreground">{description}</div>
        </div>
      </div>
      {children}
    </div>
  )
}

// FormPreview mirrors the storefront renderer (WebFormClient): fields grouped into sections rendered as fieldsets with
// title/description and the boxed style, hidden sections dropping their whole group, and layout fields shown
function FormPreview({
  name,
  fields,
  answers,
  settings,
  onChange,
}: {
  name: string
  fields: Field[]
  answers: Record<string, AnswerValue>
  settings: FormSettings
  onChange: (key: string, val: AnswerValue) => void
}) {
  const visKeys = new Set(visibleFields(fields, answers).map((f) => f.key))
  const groups: { section?: Field; fields: Field[] }[] = [{ fields: [] }]
  for (const f of fields) {
    if (f.type === 'section') groups.push({ section: f, fields: [] })
    else groups[groups.length - 1].fields.push(f)
  }

  const renderField = (f: Field, key: string) => {
    if (isLayout(f.type)) return <PreviewLayoutField key={key} field={f} />
    if (!visKeys.has(f.key)) return null
    return (
      <div key={key} className="grid gap-1.5">
        {f.type !== 'consent' && (
          <Label>
            {f.label}
            {f.required && <span className="ml-1 text-destructive">*</span>}
          </Label>
        )}
        <PreviewInput field={f} value={answers[f.key]} onChange={(v) => onChange(f.key, v)} />
        {f.helpText && <p className="text-[11px] text-muted-foreground">{f.helpText}</p>}
      </div>
    )
  }

  return (
    <div className="mx-auto w-full max-w-xl overflow-hidden rounded-xl border bg-muted/30 shadow-sm">
      {/* Browser-chrome header to signal this is the live storefront view */}
      <div className="flex items-center gap-1.5 border-b bg-muted/60 px-3 py-2">
        <span className="size-2.5 rounded-full bg-destructive/70" />
        <span className="size-2.5 rounded-full bg-warning/70" />
        <span className="size-2.5 rounded-full bg-success/70" />
        <span className="ml-2 truncate text-xs text-muted-foreground">{name || 'Untitled form'}</span>
      </div>
      <div className="grid gap-4 bg-background p-6">
        {name && <h2 className="text-xl font-semibold">{name}</h2>}
        {fields.length === 0 && (
          <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
            Add fields to see the form.
          </p>
        )}
        {groups.map((g, gi) => {
          if (g.section && !isFieldVisible(g.section, answers)) return null
          const body = g.fields.map((f, fi) => renderField(f, `${gi}.${fi}`))
          if (!g.section) {
            return body.some(Boolean) ? (
              <div key={gi} className="grid gap-4">
                {body}
              </div>
            ) : null
          }
          return (
            <fieldset key={gi} className={g.section.boxed ? 'grid gap-4 rounded-md border p-4' : 'grid gap-4'}>
              {g.section.label && <legend className="px-1 text-base font-semibold">{g.section.label}</legend>}
              {g.section.helpText && <p className="text-sm text-muted-foreground">{g.section.helpText}</p>}
              {body}
            </fieldset>
          )
        })}
        {fields.length > 0 && <Button className="w-fit" type="button">{settings.submitLabel || 'Submit'}</Button>}
      </div>
    </div>
  )
}

function PreviewLayoutField({ field }: { field: Field }) {
  if (field.type === 'divider') return <hr />
  if (field.type === 'heading') return <h3 className="text-lg font-semibold">{field.label}</h3>
  return <p className="text-sm text-muted-foreground">{field.label}</p>
}

function PreviewInput({ field, value, onChange }: { field: Field; value: AnswerValue; onChange: (v: AnswerValue) => void }) {
  const str = typeof value === 'string' ? value : ''
  switch (field.type) {
    case 'textarea':
      return <Textarea value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />
    case 'select':
      return (
        <Select value={str} onValueChange={onChange}>
          <SelectTrigger><SelectValue placeholder={field.placeholder || 'Select…'} /></SelectTrigger>
          <SelectContent>
            {(field.options ?? []).map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}
          </SelectContent>
        </Select>
      )
    case 'radio':
      return (
        <div className="grid gap-1.5">
          {(field.options ?? []).map((o) => (
            <label key={o.value} className="flex items-center gap-2 text-sm">
              <input type="radio" name={field.key} checked={str === o.value} onChange={() => onChange(o.value)} />
              {o.label}
            </label>
          ))}
        </div>
      )
    case 'checkbox_group': {
      const arr = Array.isArray(value) ? (value as string[]) : []
      return (
        <div className="grid gap-1.5">
          {(field.options ?? []).map((o) => (
            <label key={o.value} className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={arr.includes(o.value)}
                onCheckedChange={(c) => onChange(c ? [...arr, o.value] : arr.filter((x) => x !== o.value))}
              />
              {o.label}
            </label>
          ))}
        </div>
      )
    }
    case 'consent':
      return (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={value === true} onCheckedChange={(c) => onChange(!!c)} />
          {field.label}
        </label>
      )
    case 'matrix': {
      const m: MatrixValue = value && typeof value === 'object' && !Array.isArray(value) ? (value as MatrixValue) : {}
      const setCell = (rowKey: string, col: string) => {
        if (field.multi) {
          const cur = Array.isArray(m[rowKey]) ? (m[rowKey] as string[]) : []
          onChange({ ...m, [rowKey]: cur.includes(col) ? cur.filter((x) => x !== col) : [...cur, col] })
        } else {
          onChange({ ...m, [rowKey]: col })
        }
      }
      return (
        <div className="overflow-x-auto">
          <table className="text-sm">
            <thead>
              <tr>
                <th />
                {(field.columns ?? []).map((c) => (
                  <th key={c.value} className="px-2 pb-1 text-center text-xs font-normal text-muted-foreground">{c.label}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(field.rows ?? []).map((r) => (
                <tr key={r.key}>
                  <td className="py-1 pr-3">{r.label}</td>
                  {(field.columns ?? []).map((c) => {
                    const on = field.multi
                      ? Array.isArray(m[r.key]) && (m[r.key] as string[]).includes(c.value)
                      : m[r.key] === c.value
                    return (
                      <td key={c.value} className="px-2 text-center">
                        <input type={field.multi ? 'checkbox' : 'radio'} name={`${field.key}-${r.key}`} checked={!!on} onChange={() => setCell(r.key, c.value)} />
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )
    }
    case 'repeater': {
      const items: RepeaterValue = Array.isArray(value) ? (value as RepeaterValue) : []
      const setItem = (i: number, patch: Record<string, AnswerValue>) =>
        onChange(items.map((it, j) => (j === i ? { ...it, ...patch } : it)))
      const max = field.maxItems
      return (
        <div className="grid gap-2">
          {items.map((it, i) => (
            <div key={i} className="grid gap-1.5 rounded-md border p-2">
              {(field.subFields ?? []).map((sf) => (
                <div key={sf.key} className="grid gap-1">
                  <Label className="text-xs">
                    {sf.label}
                    {sf.required && <span className="ml-0.5 text-destructive">*</span>}
                  </Label>
                  <PreviewInput field={sf} value={it[sf.key]} onChange={(v) => setItem(i, { [sf.key]: v })} />
                </div>
              ))}
              <Button variant="ghost" size="sm" className="w-fit text-destructive" onClick={() => onChange(items.filter((_, j) => j !== i))}>
                <Trash2Icon className="size-3" /> Remove
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            size="sm"
            className="w-fit"
            disabled={!!max && items.length >= max}
            onClick={() => onChange([...items, {}])}
          >
            <PlusIcon className="size-4" /> Add {field.label || 'item'}
          </Button>
        </div>
      )
    }
    case 'file':
      return <Input type="file" disabled />
    case 'number':
      return <Input type="number" value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />
    case 'date':
      return <Input type="date" value={str} onChange={(e) => onChange(e.target.value)} />
    case 'time':
      return <Input type="time" value={str} onChange={(e) => onChange(e.target.value)} />
    default:
      return <Input value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />
  }
}
