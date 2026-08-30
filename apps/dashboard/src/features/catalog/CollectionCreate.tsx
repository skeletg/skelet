import { useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckCircle2Icon, LanguagesIcon, ShapesIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { slugify } from '@/lib/slug'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  filterChannelsByAllowedScopes,
  localesForAllowedChannel,
  useConfigScope,
} from '@/lib/config-scope'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { LexicalRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'
import {
  CollectionsDocument,
  CreateCollectionDocument,
} from './Collections.gql'

function isValidSlug(input: string): boolean {
  return /^[a-z0-9_-]+$/.test(input)
}

function uniqueLocales(input: string[]): string[] {
  return Array.from(new Set(input.filter(Boolean)))
}

export function CollectionCreate() {
  const navigate = useNavigate()
  const permissions = usePermissions()
  const { globalDefaultLocale } = useConfigScope()
  const allowedCatalogScopes = useMemo(
    () => permissions.allowedChannelScopes('catalog.manage'),
    [permissions],
  )

  const channelsQuery = useQuery(ChannelsDocument)
  const [createCollection, { loading: creating }] = useMutation(CreateCollectionDocument, {
    refetchQueries: [{ query: CollectionsDocument }],
  })

  const channels = useMemo(
    () => filterChannelsByAllowedScopes(channelsQuery.data?.channels ?? [], allowedCatalogScopes),
    [allowedCatalogScopes, channelsQuery.data],
  )
  const availableLocales = useMemo(() => {
    const all = channels.flatMap((sv) => localesForAllowedChannel(sv, allowedCatalogScopes))
    return uniqueLocales(all)
  }, [allowedCatalogScopes, channels])

  const draft = useDraft({
    slug: '',
    name: '',
    locale: '',
    isPublished: false,
    seoNoindex: false,
    metaTitle: '',
    metaDescription: '',
    metaKeywords: '',
    descriptionText: '',
    description: null as RichTextValue,
  }, 'New collection')
  const { slug, name, locale, isPublished, seoNoindex, metaTitle, metaDescription, metaKeywords, descriptionText, description } = draft.values
  const { setSlug, setName, setLocale, setIsPublished, setSeoNoindex, setMetaTitle, setMetaDescription, setMetaKeywords, setDescriptionText, setDescription } = draft.setters
  const [slugTouched, setSlugTouched] = useState(false)
  // Picker defaults to the system-wide default locale (if visible in this scope) or the first allowed locale - Never
  // hardcoded to a specific code Empty until availableLocales / globalDefaultLocale resolve
  if (!locale && availableLocales.length > 0) {
    setLocale(
      globalDefaultLocale && availableLocales.includes(globalDefaultLocale)
        ? globalDefaultLocale
        : availableLocales[0],
    )
  }
  const [submitError, setSubmitError] = useState<string | null>(null)

  function onNameChange(next: string) {
    setName(next)
    if (!slugTouched) setSlug(slugify(next))
  }

  function validate(): string | null {
    if (!name.trim()) return 'Name is required.'
    if (!slug.trim()) return 'Slug is required.'
    if (!isValidSlug(slug)) {
      return 'Slug can only contain lowercase letters, numbers, underscores, and hyphens.'
    }
    if (!locale.trim()) return 'Locale is required.'
    return null
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitError(null)
    const validationError = validate()
    if (validationError) {
      setSubmitError(validationError)
      return
    }

    try {
      const result = await createCollection({
        variables: {
          input: {
            slug: slug.trim(),
            locale: locale.trim(),
            name: name.trim(),
            isPublished,
            seoNoindex,
            description: descriptionText.trim() ? description : undefined,
            metaTitle: metaTitle.trim() || undefined,
            metaDescription: metaDescription.trim() || undefined,
            metaKeywords: metaKeywords.trim() || undefined,
          },
        },
      })

      const newId = result.data?.createCollection.id
      if (!newId) {
        setSubmitError('Create succeeded but no collection id was returned.')
        return
      }
      await duringCommit(() => navigate(`/collections/${newId}`))
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  const formReady = Boolean(name.trim()) && Boolean(slug.trim()) && isValidSlug(slug) && Boolean(locale.trim())

  return (
    <div className="grid gap-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="grid gap-1">
          <h1 className="text-2xl font-bold tracking-tight">New collection</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Create the collection record and first translation. Cover image, product membership,
            redirects, and metadata are managed after creation.
          </p>
        </div>
        <Badge variant="secondary" className="gap-1 self-start">
          <ShapesIcon className="size-3" />
          Catalog collection
        </Badge>
      </div>

      {channelsQuery.error && (
        <Alert variant="destructive">
          <AlertTitle>Couldn't load locale options</AlertTitle>
          <AlertDescription>{channelsQuery.error.message}</AlertDescription>
        </Alert>
      )}

      <form onSubmit={onSubmit} className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
        <div className="grid gap-4">
          <Card>
            <CardHeader>
              <CardTitle>General</CardTitle>
              <CardDescription>Slug, status, and initial translation.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="collection-name">Name</Label>
                <Input
                  id="collection-name"
                  value={name}
                  onChange={(event) => onNameChange(event.target.value)}
                  placeholder="Summer arrivals"
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="collection-slug">Slug</Label>
                <Input
                  id="collection-slug"
                  value={slug}
                  onChange={(event) => {
                    setSlugTouched(true)
                    setSlug(event.target.value)
                  }}
                  placeholder="summer-arrivals"
                />
                <p className="text-xs text-muted-foreground">
                  Used for public URLs and URL rewrites.
                </p>
              </div>

              <div className="grid gap-2">
                <Label htmlFor="collection-locale">Locale</Label>
                {availableLocales.length > 0 ? (
                  <Select value={locale} onValueChange={setLocale}>
                    <SelectTrigger id="collection-locale">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {availableLocales.map((code) => (
                        <SelectItem key={code} value={code}>
                          {code}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    id="collection-locale"
                    value={locale}
                    onChange={(event) => setLocale(event.target.value)}
                    placeholder="en"
                  />
                )}
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <label className="flex items-center justify-between gap-4 rounded-md border p-3">
                  <div className="grid gap-1">
                    <span className="text-sm font-medium">Published</span>
                    <span className="text-xs text-muted-foreground">
                      Make the collection eligible for use on channels.
                    </span>
                  </div>
                  <Switch checked={isPublished} onCheckedChange={setIsPublished} />
                </label>

                <label className="flex items-center justify-between gap-4 rounded-md border p-3">
                  <div className="grid gap-1">
                    <span className="text-sm font-medium">Noindex</span>
                    <span className="text-xs text-muted-foreground">
                      Ask search engines not to index this collection.
                    </span>
                  </div>
                  <Switch checked={seoNoindex} onCheckedChange={setSeoNoindex} />
                </label>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Description</CardTitle>
              <CardDescription>Locale-specific content stored as rich text JSON.</CardDescription>
            </CardHeader>
            <CardContent>
              <LexicalRichTextEditor
                value={description}
                fallbackText=""
                placeholder="Describe the collection and merchandising intent..."
                onChange={(next, plainText) => {
                  setDescription(next)
                  setDescriptionText(plainText)
                }}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>SEO</CardTitle>
              <CardDescription>Optional search metadata for the initial locale.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="collection-meta-title">Meta title</Label>
                <Input
                  id="collection-meta-title"
                  value={metaTitle}
                  onChange={(event) => setMetaTitle(event.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="collection-meta-description">Meta description</Label>
                <Textarea
                  id="collection-meta-description"
                  rows={4}
                  value={metaDescription}
                  onChange={(event) => setMetaDescription(event.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="collection-meta-keywords">Meta keywords</Label>
                <Input
                  id="collection-meta-keywords"
                  value={metaKeywords}
                  onChange={(event) => setMetaKeywords(event.target.value)}
                />
              </div>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Create collection</CardTitle>
            <CardDescription>Required fields and locale are validated before submit.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <div className="grid gap-3 text-sm">
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Name</span>
                {name.trim() ? <CheckCircle2Icon className="size-4 text-emerald-600" /> : <span>Required</span>}
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Slug</span>
                {slug.trim() && isValidSlug(slug) ? <CheckCircle2Icon className="size-4 text-emerald-600" /> : <span>Required</span>}
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Locale</span>
                <span className="inline-flex items-center gap-1">
                  <LanguagesIcon className="size-4 text-muted-foreground" />
                  {locale || 'Required'}
                </span>
              </div>
            </div>

            {submitError && (
              <Alert variant="destructive">
                <AlertTitle>Create failed</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            )}

            <Button type="submit" disabled={!formReady || creating} className="w-full">
              {creating ? 'Creating...' : 'Create collection'}
            </Button>
          </CardContent>
        </Card>
      </form>
    </div>
  )
}
