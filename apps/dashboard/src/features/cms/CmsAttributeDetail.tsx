import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { GripIcon, PlusIcon, Trash2Icon, Undo2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { DetailPageSection } from '@/components/layout/detail-page'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { added, changed, removed } from '@/shared/saving/diff'
import { cn } from '@/lib/utils'
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
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

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
    await deleteAttr({ variables: { id: a.id } })
    navigate('/cms/attributes', { replace: true })
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
            onClick={() => setConfirmDelete(true)}
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

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={<>Delete {attrName(a)}?</>}
        description="This permanently removes the attribute definition. It must be unassigned from all page types first."
        confirmLabel="Delete attribute"
        busyLabel="Deleting…"
        destructive
        onConfirm={onDelete}
      />
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

  // Staged, never saved directly - renames, adds and deletes wait for the header Save and its confirm dialog,
  // like the rest of the page. Names are per locale, so a staged name is only for the locale it was typed in
  const [slugs, setSlugs] = useState<Map<string, string>>(new Map())
  const [names, setNames] = useState<Map<string, string>>(new Map())
  const [adds, setAdds] = useState<string[]>([])
  const [deletes, setDeletes] = useState<Set<string>>(new Set())
  const nameOf = (v: (typeof values)[number]) => v.allContent.find((c) => c.locale === effectiveLocale)?.name ?? ''

  // Plain render-time computation - the React Compiler memoises it, and useSaveSection only re-registers when it changes
  const diff: SectionDiffEntry[] = (() => {
    const out: (SectionDiffEntry | null)[] = adds.map((slug) => added('Option', slug))
    for (const v of values) {
      if (deletes.has(v.id)) {
        out.push(removed('Option', v.slug))
        continue
      }
      out.push(changed(`Option · ${v.slug} · slug`, v.slug, slugs.get(v.id) ?? v.slug))
      if (names.has(v.id)) out.push(changed(`Option · ${v.slug} · name (${effectiveLocale})`, nameOf(v), names.get(v.id)))
    }
    return out.filter((e): e is SectionDiffEntry => e !== null)
  })()

  useSaveSection({
    id: 'options',
    title: 'Options',
    diff,
    save: async () => {
      for (const [i, slug] of adds.entries()) {
        await addAttrValue({ variables: { attributeId, slug, position: values.length + i } })
      }
      for (const v of values) {
        if (deletes.has(v.id)) continue
        const slug = slugs.get(v.id)
        if (slug !== undefined && slug.trim() && slug.trim() !== v.slug) {
          await updateValue({ variables: { id: v.id, slug: slug.trim(), position: v.position } })
        }
        const name = names.get(v.id)?.trim()
        if (effectiveLocale && name && name !== nameOf(v)) {
          await upsertValueContent({ variables: { valueId: v.id, locale: effectiveLocale, name } })
        }
      }
      for (const id of deletes) await deleteValue({ variables: { id } })
      setAdds([])
      setSlugs(new Map())
      setNames(new Map())
      setDeletes(new Set())
    },
  })

  const stage = (setter: typeof setSlugs, id: string, value: string) => setter((prev) => new Map(prev).set(id, value))
  const toggleDelete = (id: string) =>
    setDeletes((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })

  return (
    <div className="grid gap-2">
      {values.length === 0 && adds.length === 0 ? (
        <p className="text-sm text-muted-foreground">No options yet.</p>
      ) : (
        <ul className="grid gap-1">
          {values.map((v, i) => {
            const deleting = deletes.has(v.id)
            return (
              <li
                key={v.id}
                className={cn('flex items-center gap-2 rounded-sm border bg-background px-2 py-1', deleting && 'opacity-50')}
              >
                <GripIcon className="size-3 text-muted-foreground" />
                <Input
                  aria-label={`Slug of option ${v.slug}`}
                  disabled={!canManage || deleting}
                  value={slugs.get(v.id) ?? v.slug}
                  onChange={(e) => stage(setSlugs, v.id, e.target.value)}
                  className={cn('h-7 flex-1 px-2 text-xs', deleting && 'line-through')}
                />
                {effectiveLocale && (
                  <Input
                    aria-label={`Name of option ${v.slug} (${effectiveLocale})`}
                    disabled={!canManage || deleting}
                    placeholder={`name (${effectiveLocale})`}
                    value={names.get(v.id) ?? nameOf(v)}
                    onChange={(e) => stage(setNames, v.id, e.target.value)}
                    className="h-7 flex-1 px-2 text-xs"
                  />
                )}
                <span className="text-xs text-muted-foreground">#{i + 1}</span>
                {canManage && (
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={deleting ? `Keep option ${v.slug}` : `Delete option ${v.slug}`}
                    onClick={() => toggleDelete(v.id)}
                  >
                    {deleting ? <Undo2Icon className="size-3" /> : <Trash2Icon className="size-3" />}
                  </Button>
                )}
              </li>
            )
          })}
          {adds.map((slug, i) => (
            <li key={`new-${i}`} className="flex items-center gap-2 rounded-sm border border-dashed bg-background px-2 py-1">
              <PlusIcon className="size-3 text-muted-foreground" />
              <span className="flex-1 px-2 font-mono text-xs">{slug}</span>
              <span className="text-xs text-muted-foreground">unsaved</span>
              <Button
                variant="ghost"
                size="sm"
                aria-label={`Remove new option ${slug}`}
                onClick={() => setAdds((prev) => prev.filter((_, j) => j !== i))}
              >
                <Trash2Icon className="size-3" />
              </Button>
            </li>
          ))}
        </ul>
      )}
      {canManage && (
        <div className="flex gap-2">
          <Input
            aria-label="New option slug"
            value={addValue.slug}
            onChange={(e) => setAddValue({ slug: e.target.value })}
            placeholder="new-option-slug"
            className="h-7 flex-1 text-xs"
          />
          <Button
            size="sm"
            disabled={!addValue.slug.trim()}
            onClick={() => {
              setAdds((prev) => [...prev, addValue.slug.trim()])
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
