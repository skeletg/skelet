import { useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { useEditingLocale } from '@/lib/editing-locale'
import { slugify } from '@/lib/slug'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CmsPageTypesDocument, CreateCmsPageTypeDocument } from './Cms.gql'
import { emptyNewPageType, newPageTypeSchema, toNewPageTypeInput } from './schemas/pageType'

// The standalone /cms/page-types/new page - on success it opens the new type so attributes can be
// assigned next
export function CmsPageTypeCreate() {
  const navigate = useNavigate()
  const locale = useEditingLocale()
  const [slugTouched, setSlugTouched] = useState(false)
  const [createType] = useMutation(CreateCmsPageTypeDocument, { refetchQueries: [CmsPageTypesDocument] })
  const createdId = useRef<string | null>(null)

  const record = useRecordForm({
    id: 'cms-page-type',
    title: 'Page type',
    label: 'New page type',
    isEdit: false,
    baseline: emptyNewPageType,
    schema: newPageTypeSchema,
    diff: () => [],
    persist: { routeKey: `cms-page-type:new:${locale}` },
    submit: async (v) => {
      const res = await createType({ variables: { input: toNewPageTypeInput(v, locale) } })
      createdId.current = res.data?.createCMSPageType.id ?? null
    },
    onSaved: () => navigate(createdId.current ? `/cms/page-types/${createdId.current}` : '/cms/page-types'),
  })
  const { form } = record
  const name = useStore(form.store, (s) => s.values.name)

  return (
    <RecordFormPage
      record={record}
      backTo="/cms/page-types"
      backLabel="Back to page types"
      title="New page type"
      meta="Define a page template by attaching attributes. Pages are instances of a type."
      createLabel="Create page type"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/cms/page-types">Cancel</Link>
        </Button>
      }
    >
      <DetailPageSection
        title="Basics"
        description="Pick a slug pages will reference and a human-readable name. Attributes are assigned on the next screen."
      >
        <div className="grid items-start gap-4 sm:grid-cols-2">
          <form.AppField
            name="name"
            listeners={{
              onChange: ({ value }) => {
                if (!slugTouched) form.setFieldValue('slug', slugify(value))
              },
            }}
          >
            {(f) => <f.TextField id="pt-name" label={`Name (${locale})`} placeholder="Landing page" autoFocus />}
          </form.AppField>
          <form.AppField name="slug">
            {(f) => (
              <f.TextField
                id="pt-slug"
                label="Slug"
                placeholder="landing"
                onValueChange={() => setSlugTouched(true)}
                hint={!slugTouched && name ? 'Generated from the name - type here to override.' : undefined}
              />
            )}
          </form.AppField>
        </div>
        <form.AppField name="description">
          {(f) => <f.TextareaField id="pt-description" label="Description (optional)" rows={2} />}
        </form.AppField>
        <form.AppField name="icon">
          {(f) => <f.TextField id="pt-icon" label="Icon hint (optional)" placeholder="layout, file…" className="sm:max-w-xs" />}
        </form.AppField>
      </DetailPageSection>
    </RecordFormPage>
  )
}
