import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { PageBackLink } from '@/components/layout/detail-page'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useConfigScope } from '@/lib/config-scope'
import {
  CmsPageTypesDocument,
  CmsPagesDocument,
  CreateCmsPageDocument,
} from './Cms.gql'

export function CmsPageCreate() {
  const navigate = useNavigate()
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const { data: typesData, loading: typesLoading } = useQuery(CmsPageTypesDocument)
  const localeOptions = useQuery(LocalesPickerDocument).data?.locales ?? []
  const types = typesData?.cmsPageTypes ?? []

  // Allow deep-linking a prefilled type (e.g. "New page" from a page-type detail)
  const [searchParams] = useSearchParams()
  const draft = useDraft({
    pageTypeId: searchParams.get('pageTypeId') ?? '',
    slug: '',
    title: '',
    locale: scopedLocale,
  }, 'New page')
  const { pageTypeId, slug, title, locale } = draft.values
  const { setPageTypeId, setSlug, setTitle, setLocale } = draft.setters
  const [error, setError] = useState<string | null>(null)

  const [createPage, { loading }] = useMutation(CreateCmsPageDocument, {
    refetchQueries: [
      { query: CmsPagesDocument, variables: { filter: { onlyPublished: false }, limit: 200, offset: 0 } },
    ],
  })

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!pageTypeId || !slug.trim() || !title.trim() || !locale.trim()) {
      setError('Page type, slug, title, and locale are required.')
      return
    }
    try {
      const result = await createPage({
        variables: {
          input: {
            pageTypeId,
            slug: slug.trim(),
            locale: locale.trim(),
            title: title.trim(),
          },
        },
      })
      const id = result.data?.createCMSPage.id
      if (id) await duringCommit(() => navigate(`/cms/pages/${id}`))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create page.')
    }
  }

  return (
    <form onSubmit={onSubmit} className="grid max-w-2xl gap-4">
      <div className="grid gap-2">
        <PageBackLink className="justify-self-start" />
        <h1 className="text-2xl font-bold tracking-tight">New page</h1>
        <p className="text-sm text-muted-foreground">
          Pick a page type - its attributes drive the editor's fields.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Could not create page</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Basics</CardTitle>
          <CardDescription>Custom fields can be filled in after creation.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="grid gap-1.5">
            <Label>Page type</Label>
            {typesLoading ? (
              <p className="text-sm text-muted-foreground">Loading types…</p>
            ) : types.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Create a page type first under <em>CMS → Page types</em>.
              </p>
            ) : (
              <Select value={pageTypeId} onValueChange={setPageTypeId}>
                <SelectTrigger>
                  <SelectValue placeholder="Select page type…" />
                </SelectTrigger>
                <SelectContent>
                  {types.map((t) => {
                    const singletonUsed = t.isSingleton && t.pageCount > 0
                    return (
                      <SelectItem key={t.id} value={t.id} disabled={singletonUsed}>
                        {t.content?.name ?? t.slug}
                        {singletonUsed && ' - singleton, already used'}
                      </SelectItem>
                    )
                  })}
                </SelectContent>
              </Select>
            )}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="slug">Slug</Label>
            <Input id="slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="about-us" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="locale">Initial locale</Label>
            <Select value={locale} onValueChange={setLocale}>
              <SelectTrigger id="locale">
                <SelectValue placeholder="Select locale…" />
              </SelectTrigger>
              <SelectContent>
                {localeOptions.map((l) => (
                  <SelectItem key={l.code} value={l.code}>
                    <span className="font-mono text-xs">{l.code}</span> · {l.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="title">Title</Label>
            <Input id="title" value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={() => navigate('/cms/pages')}>
          Cancel
        </Button>
        <Button type="submit" disabled={loading}>
          {loading ? 'Creating…' : 'Create page'}
        </Button>
      </div>
    </form>
  )
}
