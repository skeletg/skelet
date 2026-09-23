import { useMemo, useRef } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { useEditingLocale } from '@/lib/editing-locale'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CmsPageTypesDocument, CmsPagesDocument, CreateCmsPageDocument } from './Cms.gql'
import { newPageSchema, toNewPageInput } from './schemas/newPage'

export function CmsPageCreate() {
  const navigate = useNavigate()
  const editingLocale = useEditingLocale()
  const { data: typesData, loading: typesLoading } = useQuery(CmsPageTypesDocument)
  const types = typesData?.cmsPageTypes ?? []
  const locales = useQuery(LocalesPickerDocument).data?.locales ?? []
  // A page type can be preselected by deep link - "New page" from a page type's detail
  const [searchParams] = useSearchParams()
  const presetType = searchParams.get('pageTypeId') ?? ''

  const [createPage] = useMutation(CreateCmsPageDocument, {
    refetchQueries: [{ query: CmsPagesDocument, variables: { filter: { onlyPublished: false }, limit: 200, offset: 0 } }],
  })
  const createdId = useRef<string | null>(null)

  const baseline = useMemo(() => ({ pageTypeId: presetType, slug: '', title: '', locale: editingLocale }), [presetType, editingLocale])
  const record = useRecordForm({
    id: 'cms-page',
    title: 'Page',
    label: 'New page',
    isEdit: false,
    baseline,
    schema: newPageSchema,
    diff: () => [],
    persist: { routeKey: `cms-page:new:${presetType}` },
    submit: async (v) => {
      const result = await createPage({ variables: { input: toNewPageInput(v) } })
      createdId.current = result.data?.createCMSPage.id ?? null
    },
    onSaved: () => navigate(createdId.current ? `/cms/pages/${createdId.current}` : '/cms/pages'),
  })
  const { form } = record

  return (
    <RecordFormPage
      record={record}
      backTo="/cms/pages"
      backLabel="Back to pages"
      title="New page"
      meta="Pick a page type - its attributes drive the editor's fields."
      createLabel="Create page"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/cms/pages">Cancel</Link>
        </Button>
      }
    >
      <DetailPageSection title="Basics" description="Custom fields can be filled in after creation.">
        <div className="grid max-w-2xl gap-4">
          {typesLoading ? (
            <p className="text-sm text-muted-foreground">Loading types…</p>
          ) : types.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Create a page type first under <em>CMS → Page types</em>.
            </p>
          ) : (
            <form.AppField name="pageTypeId">
              {(f) => (
                <f.SelectField
                  id="page-type"
                  label="Page type"
                  placeholder="Select page type…"
                  options={types.map((t) => {
                    // A singleton type allows one page, and it's taken
                    const singletonUsed = t.isSingleton && t.pageCount > 0
                    return {
                      value: t.id,
                      label: `${t.content?.name ?? t.slug}${singletonUsed ? ' - singleton, already used' : ''}`,
                      disabled: singletonUsed,
                    }
                  })}
                />
              )}
            </form.AppField>
          )}
          <form.AppField name="slug">{(f) => <f.TextField id="slug" label="Slug" placeholder="about-us" />}</form.AppField>
          <form.AppField name="locale">
            {(f) => (
              <f.SelectField
                id="locale"
                label="Initial locale"
                placeholder="Select locale…"
                options={locales.map((l) => ({
                  value: l.code,
                  label: (
                    <>
                      <span className="font-mono text-xs">{l.code}</span> · {l.name}
                    </>
                  ),
                }))}
              />
            )}
          </form.AppField>
          <form.AppField name="title">{(f) => <f.TextField id="title" label="Title" />}</form.AppField>
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}
