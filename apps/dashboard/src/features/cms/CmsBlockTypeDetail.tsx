import { useMemo, useRef } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'
import { DetailPageSection } from '@/components/layout/detail-page'
import { FieldError } from '@/components/form/fields'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadError } from '@/shared/errors/LoadError'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { PluginSections } from '@/plugins/detail-sections'
import { CmsBlockTypeDocument, CreateCmsBlockTypeDocument, UpdateCmsBlockTypeDocument } from './Cms.gql'
import { CUSTOM_FIELD_KINDS } from './page-blocks'
import {
  BLOCK_TYPE_LABELS,
  blockTypeDiff,
  blockTypeSchema,
  emptyFieldRow,
  toBlockTypeInput,
  toBlockTypeValues,
  type BlockTypeValues,
  type ExistingBlockType,
} from './schemas/blockType'

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
  if (!data?.cmsBlockType) return <p className="px-6 py-6 text-sm text-muted-foreground">Block type not found.</p>
  return <BlockTypeForm mode="edit" blockType={data.cmsBlockType} />
}

function BlockTypeForm({ mode, blockType }: { mode: 'create' | 'edit'; blockType?: ExistingBlockType }) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'
  const [create] = useMutation(CreateCmsBlockTypeDocument)
  const [update] = useMutation(UpdateCmsBlockTypeDocument)
  // A new type opens on its own page - the id comes back from create
  const createdId = useRef<string | null>(null)

  const baseline = useMemo(() => toBlockTypeValues(blockType), [blockType])
  const record = useRecordForm({
    id: 'block-type',
    title: 'Block type',
    label: isEdit ? `Block type · ${blockType?.name || blockType?.key || ''}` : 'New block type',
    isEdit,
    baseline,
    schema: blockTypeSchema,
    diff: blockTypeDiff,
    persist: { routeKey: `cms-block-type:${blockType?.id ?? 'new'}` },
    submit: async (value) => {
      const input = toBlockTypeInput(value)
      if (blockType) {
        await update({ variables: { input: { id: blockType.id, ...input } } })
        toast.success('Block type saved')
        return
      }
      const res = await create({ variables: { input: { key: value.key.trim(), ...input } } })
      createdId.current = res.data?.createCMSBlockType.id ?? null
      toast.success('Block type created')
    },
    onSaved: () => {
      if (!isEdit) navigate(createdId.current ? `/cms/block-types/${createdId.current}` : '/cms/block-types')
    },
  })
  const { form, draft } = record
  const clash = (field: keyof BlockTypeValues) => <ClashNote draft={draft} field={field} />
  const knownKeys = useStore(form.store, (s) => s.values.fields.map((f) => f.key.trim()).filter(Boolean))

  return (
    <RecordFormPage
      record={record}
      backTo="/cms/block-types"
      backLabel="Block types"
      title={isEdit ? blockType?.name || '(unnamed)' : 'New block type'}
      meta="Pages using this type validate against the field schema; the storefront renders it by interpolating props into the template."
      createLabel="Create block type"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/cms/block-types">Cancel</Link>
        </Button>
      }
      labels={BLOCK_TYPE_LABELS}
      after={<PluginSections target="cms-block-type" entity={blockType ?? null} mode={isEdit ? 'edit' : 'create'} />}
    >
      <DetailPageSection title="Type">
        <div className="grid items-start gap-4 sm:grid-cols-3">
          <form.AppField name="key">
            {(f) => (
              <f.TextField
                id="bt-key"
                label="Key"
                placeholder="promo_countdown"
                disabled={isEdit}
                className="font-mono"
                hint={isEdit ? 'Keys are permanent - placed blocks reference them.' : undefined}
              />
            )}
          </form.AppField>
          <form.AppField name="name">
            {(f) => <f.TextField id="bt-name" label="Name" placeholder="Promo countdown" note={clash('name')} />}
          </form.AppField>
          <form.AppField name="icon">
            {(f) => <f.TextField id="bt-icon" label="Icon (optional)" placeholder="sparkles" note={clash('icon')} />}
          </form.AppField>
        </div>
      </DetailPageSection>

      <form.Field name="fields" mode="array">
        {(rows) => (
          <DetailPageSection
            title="Fields"
            description="What editors fill in when placing the block. Each field lands in the template as a {{key}} placeholder."
            trailing={
              <Button type="button" variant="outline" size="sm" className="gap-1" onClick={() => rows.pushValue(emptyFieldRow())}>
                <PlusIcon className="size-3.5" />
                Add field
              </Button>
            }
          >
            {rows.state.value.length === 0 ? (
              <p className="rounded-md border border-dashed px-4 py-6 text-center text-xs text-muted-foreground">
                No fields yet - the block will render the template as-is.
              </p>
            ) : (
              <ul className="grid gap-3">
                {rows.state.value.map((row, i) => (
                  <li key={i} className="grid gap-3 rounded-md border p-3">
                    <div className="grid items-start gap-2 sm:grid-cols-[1fr_1fr_10rem_auto]">
                      <form.Field name={`fields[${i}].key`}>
                        {(f) => (
                          <div className="grid gap-1">
                            <Input
                              placeholder="key"
                              aria-label={`Field ${i + 1} key`}
                              className="font-mono"
                              value={f.state.value}
                              onChange={(e) => f.handleChange(e.target.value)}
                              onBlur={f.handleBlur}
                            />
                            <FieldError field={f} />
                          </div>
                        )}
                      </form.Field>
                      <form.Field name={`fields[${i}].label`}>
                        {(f) => (
                          <Input
                            placeholder="Label"
                            aria-label={`Field ${i + 1} label`}
                            value={f.state.value}
                            onChange={(e) => f.handleChange(e.target.value)}
                          />
                        )}
                      </form.Field>
                      <form.Field name={`fields[${i}].kind`}>
                        {(f) => (
                          <select
                            aria-label={`Field ${i + 1} kind`}
                            className="h-9 rounded-md border bg-background px-2 text-sm"
                            value={f.state.value}
                            onChange={(e) => f.handleChange(e.target.value)}
                          >
                            {CUSTOM_FIELD_KINDS.map((k) => (
                              <option key={k} value={k}>
                                {k}
                              </option>
                            ))}
                          </select>
                        )}
                      </form.Field>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={`Remove field ${i + 1}`}
                        className="size-9 text-muted-foreground hover:text-destructive"
                        onClick={() => rows.removeValue(i)}
                      >
                        <Trash2Icon className="size-4" />
                      </Button>
                    </div>
                    <div className="grid gap-2 sm:grid-cols-2">
                      <form.Field name={`fields[${i}].placeholder`}>
                        {(f) => (
                          <Input
                            placeholder="Placeholder (optional)"
                            aria-label={`Field ${i + 1} placeholder`}
                            value={f.state.value}
                            onChange={(e) => f.handleChange(e.target.value)}
                          />
                        )}
                      </form.Field>
                      <form.Field name={`fields[${i}].description`}>
                        {(f) => (
                          <Input
                            placeholder="Help text (optional)"
                            aria-label={`Field ${i + 1} help text`}
                            value={f.state.value}
                            onChange={(e) => f.handleChange(e.target.value)}
                          />
                        )}
                      </form.Field>
                    </div>
                    {row.kind === 'select' && (
                      <form.Field name={`fields[${i}].options`}>
                        {(f) => (
                          <Input
                            placeholder="Options, comma-separated: small, medium, large"
                            aria-label={`Field ${i + 1} options`}
                            value={f.state.value}
                            onChange={(e) => f.handleChange(e.target.value)}
                          />
                        )}
                      </form.Field>
                    )}
                    <form.Field name={`fields[${i}].required`}>
                      {(f) => (
                        <div className="flex items-center gap-2">
                          <Checkbox id={`bt-req-${i}`} checked={f.state.value} onCheckedChange={(v) => f.handleChange(!!v)} />
                          <Label htmlFor={`bt-req-${i}`} className="cursor-pointer text-xs">
                            Required
                          </Label>
                        </div>
                      )}
                    </form.Field>
                  </li>
                ))}
              </ul>
            )}
            {clash('fields')}
          </DetailPageSection>
        )}
      </form.Field>

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
          <form.AppField name="template">
            {(f) => (
              <f.TextareaField
                aria-label="HTML template"
                rows={10}
                className="font-mono text-xs"
                placeholder={'<div class="promo">\n  <strong>{{heading}}</strong>\n  <p>{{body}}</p>\n</div>'}
                note={clash('template')}
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}
