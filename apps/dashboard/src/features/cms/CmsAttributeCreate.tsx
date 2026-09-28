import { useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useEditingLocale } from '@/lib/editing-locale'
import { usePermissions } from '@/shared/auth/usePermissions'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CmsAttributesDocument, CreateCmsAttributeDocument, UpsertCmsAttributeContentDocument } from './Cms.gql'
import {
  CMS_INPUT_TYPES,
  CMS_RELATIONSHIP_TARGETS,
  emptyNewCmsAttribute,
  newCmsAttributeSchema,
  toNewCmsAttributeInput,
} from './schemas/cmsAttribute'

// The standalone /cms/attributes/new page - captures the full definition, seeds the display name for
// the locale being edited, then opens the attribute's editor
export function CmsAttributeCreate() {
  const navigate = useNavigate()
  const canManage = usePermissions().has('cms.types_manage')
  const locale = useEditingLocale()
  const refetchQueries = [{ query: CmsAttributesDocument }]
  const [createAttr] = useMutation(CreateCmsAttributeDocument, { refetchQueries })
  const [upsertContent] = useMutation(UpsertCmsAttributeContentDocument, { refetchQueries })
  const createdId = useRef<string | null>(null)

  const record = useRecordForm({
    id: 'cms-attribute',
    title: 'Attribute',
    label: 'New page attribute',
    isEdit: false,
    baseline: emptyNewCmsAttribute,
    schema: newCmsAttributeSchema,
    diff: () => [],
    persist: { routeKey: `cms-attribute:new:${locale}` },
    submit: async (v) => {
      const res = await createAttr({ variables: { input: toNewCmsAttributeInput(v) } })
      const created = res.data?.createCMSAttribute
      createdId.current = created?.id ?? null
      if (created) await upsertContent({ variables: { input: { attributeId: created.id, locale, name: v.name.trim() } } })
    },
    onSaved: () => navigate(createdId.current ? `/cms/attributes/${createdId.current}` : '/cms/attributes'),
  })
  const { form } = record
  const inputType = useStore(form.store, (s) => s.values.inputType)

  return (
    <RecordFormPage
      record={record}
      readOnly={!canManage}
      backTo="/cms/attributes"
      backLabel="Back to attributes"
      title="New attribute"
      meta="Custom field definition reused across page types. Slug + input type are immutable after creation."
      createLabel="Create attribute"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/cms/attributes">Cancel</Link>
        </Button>
      }
    >
      {!canManage && (
        <div className="px-6 py-4">
          <Alert variant="destructive">
            <AlertTitle>Not permitted</AlertTitle>
            <AlertDescription>You don't have permission to create CMS attributes.</AlertDescription>
          </Alert>
        </div>
      )}

      <DetailPageSection
        title="Definition"
        description="Pick a slug, a display name for the current locale, and the input type. Add options to select-types on the next screen."
      >
        <div className="grid gap-4">
          <form.AppField name="slug">
            {(f) => <f.TextField id="attr-slug" label="Slug" placeholder="hero_headline" autoFocus />}
          </form.AppField>
          <form.AppField name="name">
            {(f) => <f.TextField id="attr-name" label={`Display name (${locale})`} placeholder="Hero headline" />}
          </form.AppField>
          <form.AppField name="inputType">
            {(f) => <f.SelectField id="attr-input-type" label="Input type" options={CMS_INPUT_TYPES} />}
          </form.AppField>
          {inputType === 'RELATIONSHIP' && (
            <form.AppField name="target">
              {(f) => (
                <f.SelectField
                  id="attr-target"
                  label="Relationship target"
                  placeholder="Select target…"
                  options={CMS_RELATIONSHIP_TARGETS.map((t) => ({ value: t, label: t }))}
                />
              )}
            </form.AppField>
          )}
          {inputType === 'NUMBER' && (
            <form.AppField name="unit">
              {(f) => <f.TextField id="attr-unit" label="Unit (optional)" placeholder="px, %, kg…" />}
            </form.AppField>
          )}
          <form.Field name="translatable">
            {(f) => (
              <div className="flex items-center gap-2">
                <Switch id="attr-translatable" checked={f.state.value} onCheckedChange={f.handleChange} />
                <Label htmlFor="attr-translatable">Translatable</Label>
              </div>
            )}
          </form.Field>
          <form.Field name="requiredDefault">
            {(f) => (
              <div className="flex items-center gap-2">
                <Switch id="attr-required" checked={f.state.value} onCheckedChange={f.handleChange} />
                <Label htmlFor="attr-required">Required by default</Label>
              </div>
            )}
          </form.Field>
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}
