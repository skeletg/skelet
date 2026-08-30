import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
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
import { Textarea } from '@/components/ui/textarea'
import { useConfigScope } from '@/lib/config-scope'
import { slugify } from '@/lib/slug'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { BlogPostsDocument, CreateBlogPostDocument } from './Blog.gql'

export function BlogPostCreate() {
  const navigate = useNavigate()
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const draft = useDraft({
    slug: '',
    title: '',
    locale: scopedLocale,
    excerpt: '',
  }, 'New post')
  const { slug, title, locale, excerpt } = draft.values
  const { setSlug, setTitle, setLocale, setExcerpt } = draft.setters
  // slugTouched flips true the moment the user types into the slug field;
  // until then we keep deriving the slug from the title for them
  const [slugTouched, setSlugTouched] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { data: localesData } = useQuery(LocalesPickerDocument)
  const locales = localesData?.locales ?? []

  const [createPost, { loading }] = useMutation(CreateBlogPostDocument, {
    refetchQueries: [{ query: BlogPostsDocument, variables: { filter: { includeUnpublished: true }, limit: 200, offset: 0 } }],
  })

  function onTitleChange(next: string) {
    setTitle(next)
    if (!slugTouched) setSlug(slugify(next))
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!slug.trim() || !title.trim() || !locale.trim()) {
      setError('Slug, title and locale are required.')
      return
    }
    try {
      const result = await createPost({
        variables: {
          input: {
            slug: slug.trim(),
            locale: locale.trim(),
            title: title.trim(),
            excerpt: excerpt.trim() ? { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text: excerpt.trim() }] }] } } : null,
          },
        },
      })
      const id = result.data?.createBlogPost.id
      if (id) await duringCommit(() => navigate(`/blog/posts/${id}`))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create post.')
    }
  }

  return (
    <form onSubmit={onSubmit} className="grid max-w-2xl gap-4">
      <div className="grid gap-2">
        <PageBackLink className="justify-self-start" />
        <h1 className="text-2xl font-bold tracking-tight">New blog post</h1>
        <p className="text-sm text-muted-foreground">
          Start with a title - slug auto-fills, but you can override it. Body, status, categories, and tags come later.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Could not create post</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Basics</CardTitle>
          <CardDescription>The slug is permanent in URLs but a 301 redirect is created on change.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="title">Title</Label>
            <Input
              id="title"
              value={title}
              onChange={(e) => onTitleChange(e.target.value)}
              placeholder="A clear, concise title"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="slug">Slug</Label>
            <Input
              id="slug"
              value={slug}
              onChange={(e) => {
                setSlug(e.target.value)
                setSlugTouched(true)
              }}
              placeholder="hello-world"
            />
            {!slugTouched && title && (
              <p className="text-xs text-muted-foreground">Generated from the title - type here to override.</p>
            )}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="locale">Initial locale</Label>
            <Select value={locale} onValueChange={setLocale}>
              <SelectTrigger id="locale">
                <SelectValue placeholder="Pick a locale…" />
              </SelectTrigger>
              <SelectContent>
                {locales.length === 0 ? (
                  <SelectItem value={locale}>{locale}</SelectItem>
                ) : (
                  locales.map((l) => (
                    <SelectItem key={l.code} value={l.code}>
                      {l.flagEmoji ? `${l.flagEmoji} ` : ''}
                      {l.name}
                      <span className="ml-2 text-muted-foreground">({l.code})</span>
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="excerpt">Excerpt (optional)</Label>
            <Textarea id="excerpt" value={excerpt} onChange={(e) => setExcerpt(e.target.value)} rows={3} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={() => navigate('/blog/posts')}>
          Cancel
        </Button>
        <Button type="submit" disabled={loading}>
          {loading ? 'Creating…' : 'Create post'}
        </Button>
      </div>
    </form>
  )
}
