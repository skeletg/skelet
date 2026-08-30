import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { slugify } from '@/lib/slug'
import {
  CmsPageTypesDocument,
  CreateCmsPageTypeDocument,
} from './Cms.gql'

// CmsPageTypeCreate is the standalone /cms/page-types/new create page (reached from the table's "New page type" button)
// - On success it navigates to the new type's detail page so attributes can be assigned next
export function CmsPageTypeCreate() {
  const navigate = useNavigate()
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const draft = useDraft({
    slug: '',
    name: '',
    description: '',
    icon: '',
  }, 'New page type')
  const { slug, name, description, icon } = draft.values
  const { setSlug, setName, setDescription, setIcon } = draft.setters
  const [slugTouched, setSlugTouched] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [createType, { loading: creating }] = useMutation(CreateCmsPageTypeDocument, {
    refetchQueries: [{ query: CmsPageTypesDocument }],
  })

  function onNameChange(next: string) {
    setName(next)
    if (!slugTouched) setSlug(slugify(next))
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    if (!slug.trim() || !name.trim()) {
      setSubmitError('Slug and name are required.')
      return
    }
    try {
      const res = await createType({
        variables: {
          input: {
            slug: slug.trim(),
            icon: icon.trim() || null,
            isSystem: false,
            position: 0,
            locale: scopedLocale,
            name: name.trim(),
            description: description.trim() || null,
          },
        },
      })
      const id = res.data?.createCMSPageType.id
      await duringCommit(() =>
        id ? navigate(`/cms/page-types/${id}`) : navigate('/cms/page-types'),
      )
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  return (
    <DetailPageLayout>
      <form id="cms-page-type-create-form" onSubmit={onSubmit} className="flex flex-col">
        <DetailPageHeader
          backTo="/cms/page-types"
          backLabel="Back to page types"
          title="New page type"
          meta="Define a page template by attaching attributes. Pages are instances of a type."
          actions={
            <>
              <Button type="button" variant="outline" size="sm" asChild>
                <Link to="/cms/page-types">Cancel</Link>
              </Button>
              <Button type="submit" size="sm" disabled={creating || !slug.trim() || !name.trim()}>
                {creating ? 'Creating…' : 'Create page type'}
              </Button>
            </>
          }
        />

        <DetailPageBody>
          {submitError && (
            <div className="px-6 pt-4">
              <Alert variant="destructive">
                <AlertTitle>Create failed</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          )}

          <DetailPageSection
            title="Basics"
            description="Pick a slug pages will reference and a human-readable name. Attributes are assigned on the next screen."
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="pt-name">Name ({scopedLocale})</Label>
                <Input
                  id="pt-name"
                  value={name}
                  onChange={(e) => onNameChange(e.target.value)}
                  placeholder="Landing page"
                  autoFocus
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pt-slug">Slug</Label>
                <Input
                  id="pt-slug"
                  value={slug}
                  onChange={(e) => {
                    setSlug(e.target.value)
                    setSlugTouched(true)
                  }}
                  placeholder="landing"
                />
                {!slugTouched && name && (
                  <p className="text-xs text-muted-foreground">Generated from the name - type here to override.</p>
                )}
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pt-description">Description (optional)</Label>
              <Textarea
                id="pt-description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={2}
              />
            </div>
            <div className="grid gap-1.5 sm:max-w-xs">
              <Label htmlFor="pt-icon">Icon hint (optional)</Label>
              <Input
                id="pt-icon"
                value={icon}
                onChange={(e) => setIcon(e.target.value)}
                placeholder="layout, file…"
              />
            </div>
          </DetailPageSection>
        </DetailPageBody>
      </form>
    </DetailPageLayout>
  )
}
