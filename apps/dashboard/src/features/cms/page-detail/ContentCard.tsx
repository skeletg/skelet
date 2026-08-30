import { useEffect, useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { LanguagesIcon, SparklesIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { GenerateCmsPageSeoDocument, type CmsPageRowFragment } from '../Cms.gql'
import { DynamicField, type FieldValue } from '../DynamicFieldRenderer'
import { SeoPanel, type SeoSettings } from '../SeoPanel'
import { parseDocument, serializeDocument, type CmsBlock } from '../page-blocks'
import { dynamicFieldDiff, type PageTypeAttr } from './shared'

// ---------- Locale-scoped content card ----------

export function ContentCard({
  page,
  locale,
  canManage,
  canEditTranslations,
  translatableAttrs,
  blocks,
  aiEnabled,
  onTranslate,
  onSave,
  onSaveSeo,
}: {
  page: CmsPageRowFragment
  locale: string
  canManage: boolean
  canEditTranslations: boolean
  // Whether the AI assistant is usable (gates the Generate/Translate buttons)
  aiEnabled: boolean
  // Translate this locale's content from another locale (parent owns the mutation + refetch)
  onTranslate: (sourceLocale: string) => Promise<unknown>
  translatableAttrs: PageTypeAttr[]
  // The page-layout block document is OWNED by the parent (CmsPageDetail) so
  // the dedicated builder section and the live-preview panel share one source
  // of truth - ContentCard reads it for the diff + persists it on save; the
  // canvas itself lives in the "Page layout" section above
  blocks: CmsBlock[]
  onSave: (input: {
    pageId: string
    locale: string
    title: string
    // The page body is the component-built block Document (a JSON scalar)
    body: unknown
    metaTitle: string
    metaDescription: string
    data: Record<string, FieldValue>
  }) => Promise<unknown>
  // Persists the page-level SEO settings (OG image, indexing, canonical, link-following, JSON-LD) in one write - they
  // share the metadata map, so a per-field save would clobber the others - page-scoped, not locale-scoped
  onSaveSeo: (settings: SeoSettings) => Promise<unknown>
}) {
  const existing = page.allContent.find((c) => c.locale === locale)
  const [title, setTitle] = useState(existing?.title ?? '')
  const [metaTitle, setMetaTitle] = useState(existing?.metaTitle ?? '')
  const [metaDescription, setMetaDescription] = useState(existing?.metaDescription ?? '')
  const [contentData, setContentData] = useState<Record<string, FieldValue>>(
    (existing?.data as Record<string, FieldValue>) ?? {},
  )
  // Page-level SEO metadata values, read straight off the page for the SEO panel
  const seoMeta = (k: string) => page.metadata?.find((m) => m.key === k)?.value ?? ''

  // AI assistant (suggestion-only - fills the editable fields; the header Save persists)
  const [genSeo, { loading: genningSeo }] = useMutation(GenerateCmsPageSeoDocument)
  const [translating, setTranslating] = useState(false)
  const [aiErr, setAiErr] = useState<string | null>(null)
  // Other locales that have content to translate FROM
  const translateSources = page.allContent
    .filter((c) => c.locale !== locale && (c.title || c.body))
    .map((c) => c.locale)
  const [translateSource, setTranslateSource] = useState('')

  async function onGenerateSeo() {
    setAiErr(null)
    try {
      const res = await genSeo({ variables: { pageId: page.id, locale } })
      const s = res.data?.generateCMSPageSeo
      if (s?.metaTitle) setMetaTitle(s.metaTitle)
      if (s?.metaDescription) setMetaDescription(s.metaDescription)
    } catch (e) {
      setAiErr(e instanceof Error ? e.message : 'AI generation failed.')
    }
  }

  async function onRunTranslate() {
    const src = translateSource || translateSources[0]
    if (!src) return
    setAiErr(null)
    setTranslating(true)
    try {
      await onTranslate(src)
    } catch (e) {
      setAiErr(e instanceof Error ? e.message : 'Translation failed.')
    } finally {
      setTranslating(false)
    }
  }

  // Reload locale-scoped drafts when the header locale or the page
  // identity changes - Deliberately NOT keyed on the page object -
  // refetches from unrelated actions (publish toggle, another
  // section's save) would wipe staged edits mid-edit - (The block document
  // is reset by the parent's matching effect - it owns that state now.)
  useEffect(() => {
    const c = page.allContent.find((x) => x.locale === locale)
    // Re-seed local drafts from the loaded page on identity/locale change
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTitle(c?.title ?? '')
    setMetaTitle(c?.metaTitle ?? '')
    setMetaDescription(c?.metaDescription ?? '')
    setContentData((c?.data as Record<string, FieldValue>) ?? {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page.id, locale])

  const diff = useMemo<SectionDiffEntry[]>(() => {
    // GLOBAL scope renders read-only fallback content - never dirty
    if (!canEditTranslations) return []
    const c = page.allContent.find((x) => x.locale === locale)
    const out: SectionDiffEntry[] = []
    if (title !== (c?.title ?? '')) out.push({ label: 'title', before: c?.title ?? '', after: title })
    // Body diff: compare the persisted blocks (from the parsed Document) against
    // the editor's current block list - A non-block body counts as empty
    // The after side must summarize WHAT changed, not just the count - the save
    // registry drops change entries whose before === after, so a props-only edit
    // ("8 blocks" -> "8 blocks") would otherwise never mark the page dirty
    const serverBlocks = parseDocument(c?.body)?.blocks ?? []
    if (JSON.stringify(serverBlocks) !== JSON.stringify(blocks)) {
      const serverById = new Map(serverBlocks.map((b) => [b.id, JSON.stringify(b)]))
      const draftIds = new Set(blocks.map((b) => b.id))
      const addedN = blocks.filter((b) => !serverById.has(b.id)).length
      const removedN = serverBlocks.filter((b) => !draftIds.has(b.id)).length
      const editedN = blocks.filter((b) => {
        const prev = serverById.get(b.id)
        return prev !== undefined && prev !== JSON.stringify(b)
      }).length
      const parts: string[] = []
      if (addedN) parts.push(`${addedN} added`)
      if (removedN) parts.push(`${removedN} removed`)
      if (editedN) parts.push(`${editedN} edited`)
      const blocksWord = (n: number) => `${n} block${n === 1 ? '' : 's'}`
      out.push({
        label: 'body',
        before: blocksWord(serverBlocks.length),
        after: `${blocksWord(blocks.length)} (${parts.length ? parts.join(', ') : 'reordered'})`,
      })
    }
    if (metaTitle !== (c?.metaTitle ?? '')) {
      out.push({ label: 'meta title', before: c?.metaTitle ?? '', after: metaTitle })
    }
    if (metaDescription !== (c?.metaDescription ?? '')) {
      out.push({ label: 'meta description', before: c?.metaDescription ?? '', after: metaDescription })
    }
    out.push(...dynamicFieldDiff(translatableAttrs, (c?.data as Record<string, FieldValue>) ?? {}, contentData))
    return out
  }, [canEditTranslations, page, locale, translatableAttrs, title, blocks, metaTitle, metaDescription, contentData])

  useSaveSection({
    id: 'content',
    title: `Content · ${locale}`,
    diff,
    save: async () => {
      await onSave({
        pageId: page.id,
        locale,
        title,
        // Persist the block layout as the `{version:1, blocks}` Document; the
        // `body` arg is a JSON scalar so the object is sent as-is
        body: serializeDocument(blocks),
        metaTitle,
        metaDescription,
        data: contentData,
      })
    },
  })

  return (
    <div className="grid gap-4">
      {!canEditTranslations && (
        <Alert>
          <AlertTitle>Translation editing is disabled in this scope</AlertTitle>
          <AlertDescription>
            Pick a channel + locale in the header to edit translations. Below is read-only content rendered from the closest available locale.
          </AlertDescription>
        </Alert>
      )}

      {aiEnabled && canManage && canEditTranslations && translateSources.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-dashed bg-muted/30 px-3 py-2">
          <LanguagesIcon className="size-4 text-muted-foreground" />
          <span className="text-sm">
            Translate this page into <code className="font-mono">{locale}</code> from
          </span>
          <Select value={translateSource || translateSources[0]} onValueChange={setTranslateSource}>
            <SelectTrigger className="h-8 w-auto min-w-24">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {translateSources.map((loc) => (
                <SelectItem key={loc} value={loc}>
                  {loc}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button size="sm" variant="secondary" className="h-8" disabled={translating} onClick={onRunTranslate}>
            <SparklesIcon className="size-4" />
            {translating ? 'Translating…' : 'Translate'}
          </Button>
          <span className="text-xs text-muted-foreground">Overwrites this locale's content - review before saving.</span>
        </div>
      )}

      {aiErr && (
        <Alert variant="destructive">
          <AlertTitle>AI</AlertTitle>
          <AlertDescription>{aiErr}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor="title">Title</Label>
        <Input id="title" value={title} onChange={(e) => setTitle(e.target.value)} disabled={!canManage || !canEditTranslations} />
      </div>
      {translatableAttrs.map((pta) => (
        <DynamicField
          key={pta.attribute.id}
          attribute={pta.attribute}
          isRequired={pta.isRequired || pta.attribute.isRequiredDefault}
          disabled={!canManage || !canEditTranslations}
          value={contentData[pta.attribute.slug]}
          onChange={(v) => setContentData((prev) => ({ ...prev, [pta.attribute.slug]: v }))}
        />
      ))}
      <div className="grid gap-4">
        {aiEnabled && canManage && canEditTranslations && (
          <div className="flex items-center justify-between gap-2">
            <Label className="text-sm text-muted-foreground">SEO meta</Label>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-8"
              disabled={genningSeo}
              onClick={onGenerateSeo}
              title="Generate an SEO meta title + description from this page's content"
            >
              <SparklesIcon className="size-4" />
              {genningSeo ? 'Generating…' : 'Generate with AI'}
            </Button>
          </div>
        )}
        <div className="grid gap-1.5">
          <Label htmlFor="meta-title">Meta title</Label>
          <Input
            id="meta-title"
            value={metaTitle}
            onChange={(e) => setMetaTitle(e.target.value)}
            disabled={!canManage || !canEditTranslations}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="meta-description">Meta description</Label>
          <Textarea
            id="meta-description"
            value={metaDescription}
            onChange={(e) => setMetaDescription(e.target.value)}
            rows={2}
            disabled={!canManage || !canEditTranslations}
          />
        </div>
      </div>

      <SeoPanel
        title={title}
        metaTitle={metaTitle}
        metaDescription={metaDescription}
        slug={page.slug}
        urlPath={page.urlPath}
        seoNoindex={page.seoNoindex}
        publishedAt={page.publishedAt ?? null}
        blocks={blocks}
        canManage={canManage}
        currentOgImage={seoMeta('seo.ogImage')}
        currentCanonical={seoMeta('seo.canonical')}
        currentNofollow={seoMeta('seo.nofollow') === '1' || seoMeta('seo.nofollow').toLowerCase() === 'true'}
        currentJsonld={seoMeta('seo.jsonld')}
        currentSpec={seoMeta('seo.schema')}
        onSaveSeo={onSaveSeo}
      />
    </div>
  )
}
