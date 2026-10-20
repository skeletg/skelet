import { useMemo, useState } from 'react'
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
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { DetailPageSection } from '@/components/layout/detail-page'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
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
import {
  CMS_ATTRIBUTE_DEFAULTS_LABELS,
  attributeKind,
  cmsAttributeDefaultsDiff,
  cmsAttributeDefaultsSchema,
  toCmsAttributeDefaultsInput,
  toCmsAttributeDefaultsValues,
} from './schemas/cmsAttribute'

function acceptsValues(t: CmsAttributeRowFragment['inputType']): boolean {
  return t === 'SELECT' || t === 'MULTISELECT'
}

function attrName(a: CmsAttributeRowFragment): string {
  return a.content?.name?.trim() || a.allContent[0]?.name?.trim() || a.slug
}

export function CmsAttributeDetail() {
  const { id = '' } = useParams<{ id: string }>()
  // Attribute volume is low - reuse the list doc and pick by id rather than
  // adding a dedicated single-attribute query
  const { data, loading, error, refetch } = useQuery(CmsAttributesDocument)
  const attribute = data?.cmsAttributes.find((a) => a.id === id)

  if (loading && !attribute) return <LoadingState label="Loading attribute…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load attribute"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!id || !attribute) return <p className="text-sm text-muted-foreground">Attribute not found.</p>
  return <CmsAttributeForm attribute={attribute} />
}

// The default & validation fields are the page's form and go through the header Save. Option rows
// stay explicit CRUD, which the convention allows
function CmsAttributeForm({ attribute: a }: { attribute: CmsAttributeRowFragment }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')
  const { isNum, isText, isBool } = attributeKind(a.inputType)

  const [update] = useMutation(UpdateCmsAttributeDocument, {
    refetchQueries: [CmsAttributesDocument],
  })
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteAttr, { loading: deleting }] = useMutation(DeleteCmsAttributeDocument, {
    refetchQueries: [CmsAttributesDocument],
  })

  const baseline = useMemo(() => toCmsAttributeDefaultsValues(a), [a])
  const schema = useMemo(() => cmsAttributeDefaultsSchema(a.inputType), [a.inputType])
  const diff = useMemo(() => cmsAttributeDefaultsDiff(a.inputType), [a.inputType])
  const record = useRecordForm({
    id: 'attribute-defaults',
    title: 'Defaults & validation',
    label: `Attribute · ${attrName(a)}`,
    isEdit: true,
    baseline,
    schema,
    diff,
    persist: { routeKey: `cms-attribute:${a.id}` },
    submit: async (v) => {
      await update({ variables: { input: toCmsAttributeDefaultsInput(v, a.inputType, a.id) } })
    },
  })
  const { form } = record

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteAttr({ variables: { id: a.id } })
      navigate('/cms/attributes', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <RecordFormPage
      record={record}
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
        canManage && (
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
        )
      }
      createLabel="Save changes"
      labels={CMS_ATTRIBUTE_DEFAULTS_LABELS}
      readOnly={!canManage}
    >
      <DetailPageSection title="Definition" description="Slug + input type are immutable after creation.">
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
              <dt className="text-xs uppercase tracking-wide text-muted-foreground">Relationship target</dt>
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
              {!a.isTranslatable && !a.isRequiredDefault && <span className="text-sm text-muted-foreground">-</span>}
            </dd>
          </div>
        </dl>
      </DetailPageSection>

      {acceptsValues(a.inputType) && (
        <DetailPageSection title="Options" description="Selectable values for this select / multi-select attribute.">
          <AttributeValuesEditor attributeId={a.id} canManage={canManage} values={a.values} />
        </DetailPageSection>
      )}

      <DetailPageSection
        title="Default & validation"
        description="A default value seeded into new pages, and rules enforced when a page is saved."
      >
        {/* Default and validation - which rules apply depends on the input type */}
        <div className="grid max-w-lg gap-4">
          {isBool ? (
            <form.AppField name="defaultBool">
              {(f) => <f.SwitchField id="attr-default-bool" label="Default value" description={f.state.value ? 'true' : 'false'} />}
            </form.AppField>
          ) : (
            <form.AppField name="defaultValue">
              {(f) => <f.TextField id="attr-default" label="Default value" type={isNum ? 'number' : 'text'} placeholder="No default" />}
            </form.AppField>
          )}

          {(isNum || isText) && (
            <div className="grid gap-2">
              <Label className="text-sm font-medium">Validation rules</Label>
              {isNum && (
                <div className="grid grid-cols-2 items-start gap-2">
                  <form.AppField name="min">{(f) => <f.TextField aria-label="Min" type="number" placeholder="min" />}</form.AppField>
                  <form.AppField name="max">{(f) => <f.TextField aria-label="Max" type="number" placeholder="max" />}</form.AppField>
                </div>
              )}
              {isText && (
                <>
                  <div className="grid grid-cols-2 items-start gap-2">
                    <form.AppField name="minLength">
                      {(f) => <f.TextField aria-label="Min length" type="number" placeholder="min length" />}
                    </form.AppField>
                    <form.AppField name="maxLength">
                      {(f) => <f.TextField aria-label="Max length" type="number" placeholder="max length" />}
                    </form.AppField>
                  </div>
                  <form.AppField name="regex">
                    {(f) => <f.TextField aria-label="Regex" placeholder="regex pattern (optional)" className="font-mono text-xs" />}
                  </form.AppField>
                </>
              )}
            </div>
          )}
        </div>
      </DetailPageSection>

      <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {attrName(a)}?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the attribute definition. It must be unassigned from all page types first.
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
                void onDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete attribute'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </RecordFormPage>
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
    refetchQueries: [CmsAttributesDocument],
  })
  const [updateValue] = useMutation(UpdateCmsAttributeValueDocument, {
    refetchQueries: [CmsAttributesDocument],
  })
  const [deleteValue] = useMutation(DeleteCmsAttributeValueDocument, {
    refetchQueries: [CmsAttributesDocument],
  })
  const [upsertValueContent] = useMutation(UpsertCmsAttributeValueContentDocument, {
    refetchQueries: [CmsAttributesDocument],
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
