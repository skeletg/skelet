import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { GripIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  AddCmsAttributeValueDocument,
  CmsAttributesDocument,
  DeleteCmsAttributeDocument,
  DeleteCmsAttributeValueDocument,
  UpdateCmsAttributeDocument,
  UpdateCmsAttributeValueDocument,
  UpsertCmsAttributeValueContentDocument,
  type CmsAttributeRowFragment,
} from './Cms.gql'
import { useConfigScope } from '@/lib/config-scope'

function acceptsValues(t: CmsAttributeRowFragment['inputType']): boolean {
  return t === 'SELECT' || t === 'MULTISELECT'
}

function attrName(a: CmsAttributeRowFragment): string {
  return a.content?.name?.trim() || a.allContent[0]?.name?.trim() || a.slug
}

export function CmsAttributeDetail() {
  const { id = '' } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')

  // Attribute volume is low - reuse the list doc and pick by id rather than
  // adding a dedicated single-attribute query
  const { data, loading, error, refetch } = useQuery(CmsAttributesDocument)
  const attribute = data?.cmsAttributes.find((a) => a.id === id)

  const [confirmDelete, setConfirmDelete] = useState(false)

  // The defaults/validation panel stages here and the header Save applies it through the review
  // dialog rather than carrying a Save of its own. Value rows stay explicit CRUD, which the
  // convention allows
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveAllError, setSaveAllError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveAllError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveAllError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteAttr, { loading: deleting }] = useMutation(DeleteCmsAttributeDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })

  async function onDelete() {
    if (!attribute) return
    setDeleteError(null)
    try {
      await deleteAttr({ variables: { id: attribute.id } })
      navigate('/cms/attributes', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (loading) return <LoadingState label="Loading attribute…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load attribute"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!id || !attribute)
    return <p className="text-sm text-muted-foreground">Attribute not found.</p>

  const a = attribute

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/cms/attributes"
        backLabel="Back to attributes"
        title={attrName(a)}
        badges={
          <>
            <Badge variant="outline">{a.inputType.toLowerCase()}</Badge>
            {a.isTranslatable && (
              <Badge variant="secondary" className="text-xs">
                translatable
              </Badge>
            )}
            {a.isRequiredDefault && (
              <Badge variant="secondary" className="text-xs">
                required
              </Badge>
            )}
            {a.relationshipTarget && (
              <Badge variant="outline" className="text-xs">
                → {a.relationshipTarget}
              </Badge>
            )}
          </>
        }
        meta={<code className="font-mono">{a.slug}</code>}
        actions={
          canManage ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => {
                  setDeleteError(null)
                  setConfirmDelete(true)
                }}
                disabled={deleting}
              >
                <Trash2Icon className="size-4" />
                Delete
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={!isDirty || savingAll}
                onClick={() => setConfirmingSave(true)}
              >
                {savingAll ? 'Saving…' : 'Save changes'}
              </Button>
            </>
          ) : null
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Definition"
          description="Slug + input type are immutable after creation."
        >
          <dl className="grid gap-3 sm:grid-cols-2">
            <div className="grid gap-1">
              <dt className="text-xs uppercase tracking-wide text-muted-foreground">Display name</dt>
              <dd className="text-sm font-medium">{attrName(a)}</dd>
            </div>
            <div className="grid gap-1">
              <dt className="text-xs uppercase tracking-wide text-muted-foreground">Slug</dt>
              <dd className="font-mono text-sm text-muted-foreground">{a.slug}</dd>
            </div>
            <div className="grid gap-1">
              <dt className="text-xs uppercase tracking-wide text-muted-foreground">Input type</dt>
              <dd className="text-sm">{a.inputType.toLowerCase()}</dd>
            </div>
            {a.unit && (
              <div className="grid gap-1">
                <dt className="text-xs uppercase tracking-wide text-muted-foreground">Unit</dt>
                <dd className="text-sm">{a.unit}</dd>
              </div>
            )}
            {a.relationshipTarget && (
              <div className="grid gap-1">
                <dt className="text-xs uppercase tracking-wide text-muted-foreground">
                  Relationship target
                </dt>
                <dd className="text-sm">{a.relationshipTarget}</dd>
              </div>
            )}
            <div className="grid gap-1">
              <dt className="text-xs uppercase tracking-wide text-muted-foreground">Flags</dt>
              <dd className="flex flex-wrap gap-1.5">
                {a.isTranslatable && (
                  <Badge variant="secondary" className="text-xs">
                    translatable
                  </Badge>
                )}
                {a.isRequiredDefault && (
                  <Badge variant="secondary" className="text-xs">
                    required by default
                  </Badge>
                )}
                {!a.isTranslatable && !a.isRequiredDefault && (
                  <span className="text-sm text-muted-foreground">-</span>
                )}
              </dd>
            </div>
          </dl>
        </DetailPageSection>

        {acceptsValues(a.inputType) && (
          <DetailPageSection
            title="Options"
            description="Selectable values for this select / multi-select attribute."
          >
            <AttributeValuesEditor attributeId={a.id} canManage={canManage} values={a.values} />
          </DetailPageSection>
        )}

        <DetailPageSection
          title="Default & validation"
          description="A default value seeded into new pages, and rules enforced when a page is saved."
        >
          <AttributeDefaultsValidationPanel attribute={a} canManage={canManage} />
        </DetailPageSection>
      </DetailPageBody>

      <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {attrName(a)}?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the attribute definition. It must be unassigned from all page
              types first.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                onDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete attribute'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Attribute · ${attrName(a)}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveAllError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveAllError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

function AttributeValuesEditor({
  attributeId,
  canManage,
  values,
}: {
  attributeId: string
  canManage: boolean
  values: CmsAttributeRowFragment['values']
}) {
  const { effectiveLocale } = useConfigScope()
  const [addValue, setAddValue] = useState({ slug: '' })
  const [addAttrValue] = useMutation(AddCmsAttributeValueDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })
  const [updateValue] = useMutation(UpdateCmsAttributeValueDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })
  const [deleteValue] = useMutation(DeleteCmsAttributeValueDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })
  const [upsertValueContent] = useMutation(UpsertCmsAttributeValueContentDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })

  return (
    <div className="grid gap-2">
      {values.length === 0 ? (
        <p className="text-sm text-muted-foreground">No options yet.</p>
      ) : (
        <ul className="grid gap-1">
          {values.map((v, i) => (
            <li
              key={v.id}
              className="flex items-center gap-2 rounded-sm border bg-background px-2 py-1"
            >
              <GripIcon className="size-3 text-muted-foreground" />
              <Input
                disabled={!canManage}
                onBlur={(e) =>
                  e.target.value !== v.slug &&
                  updateValue({ variables: { id: v.id, slug: e.target.value, position: v.position } })
                }
                className="h-7 flex-1 px-2 text-xs"
                defaultValue={v.slug}
              />
              {effectiveLocale && (
                <Input
                  key={`${v.id}-${effectiveLocale}`}
                  disabled={!canManage}
                  placeholder={`name (${effectiveLocale})`}
                  defaultValue={v.allContent.find((c) => c.locale === effectiveLocale)?.name ?? ''}
                  onBlur={(e) => {
                    const next = e.target.value.trim()
                    const cur = v.allContent.find((c) => c.locale === effectiveLocale)?.name ?? ''
                    if (next && next !== cur) {
                      upsertValueContent({ variables: { valueId: v.id, locale: effectiveLocale, name: next } })
                    }
                  }}
                  className="h-7 flex-1 px-2 text-xs"
                />
              )}
              <span className="text-xs text-muted-foreground">#{i + 1}</span>
              {canManage && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => deleteValue({ variables: { id: v.id } })}
                >
                  <Trash2Icon className="size-3" />
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      {canManage && (
        <div className="flex gap-2">
          <Input
            value={addValue.slug}
            onChange={(e) => setAddValue({ slug: e.target.value })}
            placeholder="new-option-slug"
            className="h-7 flex-1 text-xs"
          />
          <Button
            size="sm"
            disabled={!addValue.slug.trim()}
            onClick={async () => {
              await addAttrValue({
                variables: { attributeId, slug: addValue.slug.trim(), position: values.length },
              })
              setAddValue({ slug: '' })
            }}
          >
            <PlusIcon className="size-3" />
            Add
          </Button>
        </div>
      )}
    </div>
  )
}

// AttributeDefaultsValidationPanel edits the attribute's default value (seeded into new pages) and validation rules
// (enforced server-side on save), keyed by input type - Both ride the UpdateCMSAttribute mutation's partial update
function AttributeDefaultsValidationPanel({
  attribute: a,
  canManage,
}: {
  attribute: CmsAttributeRowFragment
  canManage: boolean
}) {
  const [update] = useMutation(UpdateCmsAttributeDocument, {
    refetchQueries: [{ query: CmsAttributesDocument }],
  })
  const t = a.inputType
  const isNum = t === 'NUMBER'
  const isText = t === 'TEXT' || t === 'TEXTAREA' || t === 'URL' || t === 'RICHTEXT'
  const isBool = t === 'BOOLEAN'
  const supportsValidation = isNum || isText
  const v = (a.validation ?? {}) as Record<string, unknown>

  const str = (x: unknown) => (x == null ? '' : String(x))
  const [def, setDef] = useState(str(a.defaultValue))
  const [defBool, setDefBool] = useState(a.defaultValue === true)
  const [min, setMin] = useState(str(v.min))
  const [max, setMax] = useState(str(v.max))
  const [minLength, setMinLength] = useState(str(v.minLength))
  const [maxLength, setMaxLength] = useState(str(v.maxLength))
  const [regex, setRegex] = useState(str(v.regex))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [ok, setOk] = useState(false)

  useSaveSection({
    id: 'attribute-defaults',
    title: 'Defaults & validation',
    diff: fieldDiff(
      {
        def: str(a.defaultValue), defBool: a.defaultValue === true, min: str(v.min), max: str(v.max),
        minLength: str(v.minLength), maxLength: str(v.maxLength), regex: str(v.regex),
      },
      { def, defBool, min, max, minLength, maxLength, regex },
      [
        { label: isBool ? 'Default' : 'Default value', value: (o) => (isBool ? o.defBool : o.def) },
        { label: 'Min', value: (o) => o.min },
        { label: 'Max', value: (o) => o.max },
        { label: 'Min length', value: (o) => o.minLength },
        { label: 'Max length', value: (o) => o.maxLength },
        { label: 'Regex', value: (o) => o.regex },
      ],
    ),
    save: () => save(),
  })

  async function save() {
    setError(null)
    setOk(false)
    setSaving(true)
    try {
      const validation: Record<string, unknown> = {}
      if (isNum) {
        if (min.trim() !== '') validation.min = Number(min)
        if (max.trim() !== '') validation.max = Number(max)
      }
      if (isText) {
        if (minLength.trim() !== '') validation.minLength = Number(minLength)
        if (maxLength.trim() !== '') validation.maxLength = Number(maxLength)
        if (regex.trim() !== '') validation.regex = regex.trim()
      }
      let defaultValue: unknown = null
      if (isBool) defaultValue = defBool
      else if (isNum) defaultValue = def.trim() === '' ? null : Number(def)
      else if (def.trim() !== '') defaultValue = def
      await update({ variables: { input: { id: a.id, validation, defaultValue } } })
      setOk(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
      throw e // the header dialog keeps itself open and surfaces the message
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="grid max-w-lg gap-4">
      <div className="grid gap-1.5">
        <Label className="text-sm font-medium">Default value</Label>
        {isBool ? (
          <div className="flex items-center gap-2">
            <Switch checked={defBool} onCheckedChange={setDefBool} disabled={!canManage} />
            <span className="text-xs text-muted-foreground">{defBool ? 'true' : 'false'}</span>
          </div>
        ) : (
          <Input
            type={isNum ? 'number' : 'text'}
            value={def}
            onChange={(e) => setDef(e.target.value)}
            placeholder="No default"
            disabled={!canManage}
            className="h-9"
          />
        )}
      </div>

      {supportsValidation && (
        <div className="grid gap-2">
          <Label className="text-sm font-medium">Validation rules</Label>
          {isNum && (
            <div className="flex gap-2">
              <Input type="number" value={min} onChange={(e) => setMin(e.target.value)} placeholder="min" disabled={!canManage} className="h-9" />
              <Input type="number" value={max} onChange={(e) => setMax(e.target.value)} placeholder="max" disabled={!canManage} className="h-9" />
            </div>
          )}
          {isText && (
            <>
              <div className="flex gap-2">
                <Input type="number" value={minLength} onChange={(e) => setMinLength(e.target.value)} placeholder="min length" disabled={!canManage} className="h-9" />
                <Input type="number" value={maxLength} onChange={(e) => setMaxLength(e.target.value)} placeholder="max length" disabled={!canManage} className="h-9" />
              </div>
              <Input value={regex} onChange={(e) => setRegex(e.target.value)} placeholder="regex pattern (optional)" disabled={!canManage} className="h-9 font-mono text-xs" />
            </>
          )}
        </div>
      )}

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          {/* No per-panel Save - the page header applies every staged panel through one dialog */}
          {saving ? <span className="text-xs text-muted-foreground">Saving…</span> : null}
          {ok && <span className="text-xs text-emerald-600">Saved.</span>}
        </div>
      )}
    </div>
  )
}
