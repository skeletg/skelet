import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { LoadError } from '@/shared/errors/LoadError'
import { PluginSections } from '@/plugins/detail-sections'

import {
  CmsBlockTypeDocument,
  CreateCmsBlockTypeDocument,
  UpdateCmsBlockTypeDocument,
} from './Cms.gql'
import { CUSTOM_FIELD_KINDS, parseCustomFields, type CmsBlockField } from './page-blocks'

// Editable subset of CmsBlockField - what the custom-block editor rows carry
type FieldDraft = {
  key: string
  label: string
  kind: string
  required: boolean
  placeholder: string
  description: string
  options: string // comma-separated values for kind=select
}

function toDrafts(fields: CmsBlockField[]): FieldDraft[] {
  return fields.map((f) => ({
    key: f.key,
    label: f.label,
    kind: f.kind,
    required: !!f.required,
    placeholder: f.placeholder ?? '',
    description: f.description ?? '',
    options: (f.options ?? []).map((o) => o.value).join(', '),
  }))
}

function fromDrafts(drafts: FieldDraft[]): CmsBlockField[] {
  return drafts
    .filter((d) => d.key.trim())
    .map((d) => ({
      key: d.key.trim(),
      label: d.label.trim() || d.key.trim(),
      kind: (CUSTOM_FIELD_KINDS as readonly string[]).includes(d.kind) ? (d.kind as CmsBlockField['kind']) : 'text',
      ...(d.required ? { required: true } : {}),
      ...(d.placeholder.trim() ? { placeholder: d.placeholder.trim() } : {}),
      ...(d.description.trim() ? { description: d.description.trim() } : {}),
      ...(d.kind === 'select' && d.options.trim()
        ? { options: d.options.split(',').map((s) => s.trim()).filter(Boolean).map((v) => ({ value: v, label: v })) }
        : {}),
    }))
}

export function CmsBlockTypeCreate() {
  return <BlockTypeForm mode="create" />
}

export function CmsBlockTypeDetail() {
  const { id = '' } = useParams<{ id: string }>()
  const { data, loading, error, refetch } = useQuery(CmsBlockTypeDocument, { variables: { id }, skip: !id })
  if (loading && !data) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading block type…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError title="Couldn't load block type" message={error.message} onRetry={() => void refetch()} retrying={loading} />
      </div>
    )
  }
  if (!data?.cmsBlockType) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Block type not found.</p>
  }
  return <BlockTypeForm mode="edit" blockType={data.cmsBlockType} />
}

function BlockTypeForm({
  mode,
  blockType,
}: {
  mode: 'create' | 'edit'
  blockType?: {
    id: string
    key: string
    name: string
    icon: string
    fields: unknown
    htmlTemplate: string
  }
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const [key, setKey] = useState(blockType?.key ?? '')
  const [name, setName] = useState(blockType?.name ?? '')
  const [icon, setIcon] = useState(blockType?.icon ?? '')
  const [template, setTemplate] = useState(blockType?.htmlTemplate ?? '')
  const [drafts, setDrafts] = useState<FieldDraft[]>(() => toDrafts(parseCustomFields(blockType?.fields)))

  const [create, { loading: creating }] = useMutation(CreateCmsBlockTypeDocument)
  const [update, { loading: updating }] = useMutation(UpdateCmsBlockTypeDocument)
  const saving = creating || updating

  const knownKeys = useMemo(() => drafts.map((d) => d.key.trim()).filter(Boolean), [drafts])

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Field drafts diff as their key list - a faithful summary of the staged array, and the save
  // thunk reads live state so a summarised diff can't persist a stale draft
  const draft = { name, icon, template, fields: knownKeys.join(', ') }
  const original = {
    name: blockType?.name ?? '',
    icon: blockType?.icon ?? '',
    template: blockType?.htmlTemplate ?? '',
    fields: toDrafts(parseCustomFields(blockType?.fields)).map((d) => d.key.trim()).filter(Boolean).join(', '),
  }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const blockTypeDiff = useMemo(
    () =>
      isEdit
        ? fieldDiff(original, draft, [
            { label: 'Name', value: (o) => o.name },
            { label: 'Icon', value: (o) => o.icon },
            { label: 'HTML template', value: (o) => o.template },
            { label: 'Fields', value: (o) => o.fields },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isEdit, originalKey, draftKey],
  )

  useSaveSection({ id: 'block-type', title: 'Block type', diff: blockTypeDiff, save: () => applyChanges() })

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  async function applyChanges() {
    const fields = fromDrafts(drafts)
    try {
      if (isEdit && blockType) {
        await update({ variables: { input: { id: blockType.id, name, icon, fields, htmlTemplate: template } } })
        toast.success('Block type saved')
      } else {
        const res = await create({ variables: { input: { key: key.trim(), name, icon, fields, htmlTemplate: template } } })
        toast.success('Block type created')
        const newId = res.data?.createCMSBlockType.id
        navigate(newId ? `/cms/block-types/${newId}` : '/cms/block-types')
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Save failed')
      throw err // the dialog keeps itself open and surfaces the message
    }
  }

  function patchDraft(i: number, patch: Partial<FieldDraft>) {
    setDrafts((prev) => prev.map((d, idx) => (idx === i ? { ...d, ...patch } : d)))
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/cms/block-types"
        backLabel="Block types"
        title={isEdit ? blockType?.name || '(unnamed)' : 'New block type'}
        meta="Pages using this type validate against the field schema; the storefront renders it by interpolating props into the template."
        actions={
          <>
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to="/cms/block-types">Cancel</Link>
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={saving || (isEdit && !isDirty)}
              onClick={() => (isEdit ? setConfirmingSave(true) : void applyChanges())}
            >
              {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create block type'}
            </Button>
          </>
        }
      />

      <div>
        <DetailPageBody>
          <DetailPageSection title="Type">
            <div className="grid items-start gap-4 sm:grid-cols-3">
              <div className="grid gap-2">
                <Label htmlFor="bt-key">Key</Label>
                <Input
                  id="bt-key"
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  placeholder="promo_countdown"
                  required
                  disabled={isEdit}
                  className="font-mono"
                />
                {isEdit && <p className="text-xs text-muted-foreground">Keys are permanent - placed blocks reference them.</p>}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="bt-name">Name</Label>
                <Input id="bt-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Promo countdown" required />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="bt-icon">Icon (optional)</Label>
                <Input id="bt-icon" value={icon} onChange={(e) => setIcon(e.target.value)} placeholder="sparkles" />
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Fields"
            description="What editors fill in when placing the block. Each field lands in the template as a {{key}} placeholder."
            trailing={
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="gap-1"
                onClick={() => setDrafts((p) => [...p, { key: '', label: '', kind: 'text', required: false, placeholder: '', description: '', options: '' }])}
              >
                <PlusIcon className="size-3.5" />
                Add field
              </Button>
            }
          >
            {drafts.length === 0 ? (
              <p className="rounded-md border border-dashed px-4 py-6 text-center text-xs text-muted-foreground">
                No fields yet - the block will render the template as-is.
              </p>
            ) : (
              <ul className="grid gap-3">
                {drafts.map((d, i) => (
                  <li key={i} className="grid gap-3 rounded-md border p-3">
                    <div className="grid gap-2 sm:grid-cols-[1fr_1fr_10rem_auto]">
                      <Input placeholder="key" className="font-mono" value={d.key} onChange={(e) => patchDraft(i, { key: e.target.value })} />
                      <Input placeholder="Label" value={d.label} onChange={(e) => patchDraft(i, { label: e.target.value })} />
                      <select
                        className="h-9 rounded-md border bg-background px-2 text-sm"
                        value={d.kind}
                        onChange={(e) => patchDraft(i, { kind: e.target.value })}
                      >
                        {CUSTOM_FIELD_KINDS.map((k) => (
                          <option key={k} value={k}>{k}</option>
                        ))}
                      </select>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="size-9 text-muted-foreground hover:text-destructive"
                        onClick={() => setDrafts((p) => p.filter((_, idx) => idx !== i))}
                      >
                        <Trash2Icon className="size-4" />
                      </Button>
                    </div>
                    <div className="grid gap-2 sm:grid-cols-2">
                      <Input placeholder="Placeholder (optional)" value={d.placeholder} onChange={(e) => patchDraft(i, { placeholder: e.target.value })} />
                      <Input placeholder="Help text (optional)" value={d.description} onChange={(e) => patchDraft(i, { description: e.target.value })} />
                    </div>
                    {d.kind === 'select' && (
                      <Input placeholder="Options, comma-separated: small, medium, large" value={d.options} onChange={(e) => patchDraft(i, { options: e.target.value })} />
                    )}
                    <div className="flex items-center gap-2">
                      <Checkbox id={`bt-req-${i}`} checked={d.required} onCheckedChange={(v) => patchDraft(i, { required: !!v })} />
                      <Label htmlFor={`bt-req-${i}`} className="cursor-pointer text-xs">Required</Label>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </DetailPageSection>

          <DetailPageSection
            title="HTML template"
            description="Rendered on the storefront with {{key}} placeholders swapped for the block's props - values are HTML-escaped and the output sanitized, so scripts and event handlers are stripped."
          >
            <div className="grid gap-2">
              {knownKeys.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                  Available:
                  {knownKeys.map((k) => (
                    <Badge key={k} variant="outline" className="font-mono">{`{{${k}}}`}</Badge>
                  ))}
                </div>
              )}
              <Textarea
                rows={10}
                className="font-mono text-xs"
                placeholder={'<div class="promo">\n  <strong>{{heading}}</strong>\n  <p>{{body}}</p>\n</div>'}
                value={template}
                onChange={(e) => setTemplate(e.target.value)}
              />
            </div>
          </DetailPageSection>
        </DetailPageBody>
      </div>
      <DetailPageBody className="pb-0">
        <PluginSections target="cms-block-type" entity={blockType ?? null} mode={isEdit ? 'edit' : 'create'} />
      </DetailPageBody>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Block type · ${blockType?.name || blockType?.key || ''}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}
