import { useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { filterChannelsByAllowedScopes, localesForAllowedChannel, useConfigScope } from '@/lib/config-scope'
import { slugify } from '@/lib/slug'
import { usePermissions } from '@/shared/auth/usePermissions'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { FormRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'
import { CollectionsDocument, CreateCollectionDocument } from './Collections.gql'
import { emptyNewCollection, newCollectionSchema, toNewCollectionInput } from './schemas/newCollection'

export function CollectionCreate() {
  const navigate = useNavigate()
  const permissions = usePermissions()
  const { globalDefaultLocale } = useConfigScope()
  const allowedScopes = useMemo(() => permissions.allowedChannelScopes('catalog.manage'), [permissions])
  const channelsQuery = useQuery(ChannelsDocument)
  const channels = useMemo(
    () => filterChannelsByAllowedScopes(channelsQuery.data?.channels ?? [], allowedScopes),
    [allowedScopes, channelsQuery.data],
  )
  // Only the locales this admin may write catalog content in
  const locales = useMemo(
    () => [...new Set(channels.flatMap((c) => localesForAllowedChannel(c, allowedScopes)).filter(Boolean))],
    [allowedScopes, channels],
  )
  const defaultLocale = globalDefaultLocale && locales.includes(globalDefaultLocale) ? globalDefaultLocale : (locales[0] ?? '')

  const [slugTouched, setSlugTouched] = useState(false)
  const [createCollection] = useMutation(CreateCollectionDocument, { refetchQueries: [{ query: CollectionsDocument }] })
  const createdId = useRef<string | null>(null)
  const schema = useMemo(() => newCollectionSchema(defaultLocale), [defaultLocale])

  const record = useRecordForm({
    id: 'collection',
    title: 'Collection',
    label: 'New collection',
    isEdit: false,
    baseline: emptyNewCollection,
    schema,
    diff: () => [],
    persist: { routeKey: 'collection:new' },
    submit: async (v) => {
      const result = await createCollection({ variables: { input: toNewCollectionInput(v, defaultLocale) } })
      const id = result.data?.createCollection.id
      if (!id) throw new Error('Create succeeded but no collection id was returned.')
      createdId.current = id
    },
    onSaved: () => navigate(`/collections/${createdId.current}`),
  })
  const { form } = record
  const values = useStore(form.store, (s) => s.values)

  return (
    <RecordFormPage
      record={record}
      backTo="/collections"
      backLabel="Back to collections"
      title="New collection"
      meta="Create the collection record and first translation. Cover image, product membership, redirects, and metadata are managed after creation."
      createLabel="Create collection"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/collections">Cancel</Link>
        </Button>
      }
    >
      {channelsQuery.error && (
        <div className="px-6 py-4">
          <Alert variant="destructive">
            <AlertTitle>Couldn't load locale options</AlertTitle>
            <AlertDescription>{channelsQuery.error.message}</AlertDescription>
          </Alert>
        </div>
      )}

      <DetailPageSection title="General" description="Slug, status, and initial translation.">
        <div className="grid max-w-2xl gap-4">
          <form.AppField
            name="name"
            listeners={{
              onChange: ({ value }) => {
                if (!slugTouched) form.setFieldValue('slug', slugify(value))
              },
            }}
          >
            {(f) => <f.TextField id="collection-name" label="Name" placeholder="Summer arrivals" />}
          </form.AppField>
          <form.AppField name="slug">
            {(f) => (
              <f.TextField
                id="collection-slug"
                label="Slug"
                placeholder="summer-arrivals"
                onValueChange={() => setSlugTouched(true)}
                hint="Used for public URLs and URL rewrites."
              />
            )}
          </form.AppField>
          <form.AppField name="locale">
            {(f) =>
              locales.length > 0 ? (
                <f.SelectField
                  id="collection-locale"
                  label="Locale"
                  // Nothing picked yet reads as the default - that's what gets sent
                  placeholder={defaultLocale}
                  options={locales.map((code) => ({ value: code, label: code }))}
                />
              ) : (
                <f.TextField id="collection-locale" label="Locale" placeholder="en" />
              )
            }
          </form.AppField>
          <div className="grid items-start gap-4 sm:grid-cols-2">
            <form.AppField name="isPublished">
              {(f) => <f.SwitchField label="Published" description="Make the collection eligible for use on channels." />}
            </form.AppField>
            <form.AppField name="seoNoindex">
              {(f) => <f.SwitchField label="Noindex" description="Ask search engines not to index this collection." />}
            </form.AppField>
          </div>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Description" description="Locale-specific content stored as rich text JSON.">
        <FormRichTextEditor
          value={values.description as RichTextValue}
          fallbackText=""
          placeholder="Describe the collection and merchandising intent..."
          onChange={(next, plainText) => {
            form.setFieldValue('description', next)
            form.setFieldValue('descriptionText', plainText)
          }}
        />
      </DetailPageSection>

      <DetailPageSection title="SEO" description="Optional search metadata for the initial locale.">
        <div className="grid max-w-2xl gap-4">
          <form.AppField name="metaTitle">{(f) => <f.TextField id="collection-meta-title" label="Meta title" />}</form.AppField>
          <form.AppField name="metaDescription">
            {(f) => <f.TextareaField id="collection-meta-description" label="Meta description" rows={4} />}
          </form.AppField>
          <form.AppField name="metaKeywords">
            {(f) => <f.TextField id="collection-meta-keywords" label="Meta keywords" />}
          </form.AppField>
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}
