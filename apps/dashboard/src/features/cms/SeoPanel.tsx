import { useEffect, useMemo, useState, useId } from 'react'
import { AlertTriangleIcon, CheckCircle2Icon, ImageIcon, XCircleIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
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
import { MediaImageInput } from '@/features/media/MediaImageInput'
import { assetPreviewUrl, type AssetRef } from '@/features/media/asset-ref'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { type CmsBlock } from './page-blocks'
import { buildSchemaFromSpec, type SchemaSpec, type SchemaType } from './schema-jsonld'
import { firstBlockImage, runSeoLint, truncate, type LintLevel } from './seo-analysis'
import { STOREFRONT_ORIGIN } from '@/lib/app-origins'

// Absolute URL for the raw-JSON dump - matches how the storefront resolves it at render
function absUrl(pathOrUrl: string): string {
  if (/^https?:\/\//.test(pathOrUrl)) return pathOrUrl
  return STOREFRONT_ORIGIN + (pathOrUrl.startsWith('/') ? pathOrUrl : '/' + pathOrUrl)
}

type SchemaMode = 'none' | SchemaType | 'custom'

function schemaModeLabel(m: SchemaMode): string {
  if (m === 'none') return 'none'
  if (m === 'custom') return 'custom JSON'
  return m
}

function parseSpec(raw: string): SchemaSpec | null {
  if (!raw.trim()) return null
  try {
    const s = JSON.parse(raw) as SchemaSpec
    return s && typeof s === 'object' ? s : null
  } catch {
    return null
  }
}

// Raw JSON is the custom escape hatch, a spec means a generated type, neither means off
function initialSchemaMode(rawJson: string, spec: SchemaSpec | null): SchemaMode {
  if (rawJson.trim()) return 'custom'
  if (spec) return spec.type === 'Article' ? 'Article' : 'WebPage'
  return 'none'
}

type ImageValue = string | AssetRef

function LintIcon({ level }: { level: LintLevel }) {
  if (level === 'fail') return <XCircleIcon className="mt-0.5 size-4 shrink-0 text-destructive" />
  if (level === 'warn') return <AlertTriangleIcon className="mt-0.5 size-4 shrink-0 text-warning" />
  return <CheckCircle2Icon className="mt-0.5 size-4 shrink-0 text-success" />
}

function SerpPreview({ title, description, urlPath }: { title: string; description: string; urlPath: string }) {
  const path = urlPath.startsWith('/') ? urlPath : `/${urlPath}`
  return (
    <div className="rounded-md border bg-background p-3">
      <div className="truncate text-xs text-muted-foreground">yourstore.com{path}</div>
      <div className="truncate text-[18px] leading-tight text-[#1a0dab] dark:text-[#8ab4f8]">
        {truncate(title || 'Untitled page', 60)}
      </div>
      <div className="mt-0.5 line-clamp-2 text-sm text-muted-foreground">
        {truncate(description, 160) || 'No meta description - search engines will pull text from the page.'}
      </div>
    </div>
  )
}

function SocialPreview({
  image,
  title,
  description,
  usingFallback,
}: {
  image: string
  title: string
  description: string
  usingFallback: boolean
}) {
  return (
    <div className="overflow-hidden rounded-md border bg-background">
      {image ? (
        <img src={image} alt="" className="aspect-[1.91/1] w-full bg-muted object-cover" />
      ) : (
        <div className="flex aspect-[1.91/1] w-full items-center justify-center bg-muted text-muted-foreground">
          <ImageIcon className="size-8" />
        </div>
      )}
      <div className="border-t p-2.5">
        <div className="truncate text-sm font-medium">{title || 'Untitled page'}</div>
        <div className="line-clamp-2 text-xs text-muted-foreground">{description || '-'}</div>
        {usingFallback && image && (
          <div className="mt-1 text-[11px] text-muted-foreground">Falling back to the first page image (no OG image set).</div>
        )}
      </div>
    </div>
  )
}

// Saved together, not per-field - updateCMSPage replaces the whole metadata map, so separate save sections would clobber each other's seo.* keys
export type SeoSettings = {
  ogImage: string
  noindex: boolean
  canonical: string
  nofollow: boolean
  // raw JSON-LD (custom mode) and the builder spec (generated mode) - at most one is set,
  // the storefront prefers raw when both somehow are
  jsonld: string
  schema: string
}

// SeoPanel: live search + social previews, an on-page lint off the editor's current
// title/meta/block state, and the page-level SEO settings (OG image, indexing, canonical,
// link-following, JSON-LD) - all saved in one section since they share the metadata map,
// while the previews/lint stay pure UI off the live state
export function SeoPanel({
  title,
  metaTitle,
  metaDescription,
  slug,
  urlPath,
  seoNoindex,
  blocks,
  canManage,
  publishedAt,
  currentOgImage,
  currentCanonical,
  currentNofollow,
  currentJsonld,
  currentSpec,
  onSaveSeo,
}: {
  title: string
  metaTitle: string
  metaDescription: string
  slug: string
  urlPath: string
  seoNoindex: boolean
  publishedAt: string | null
  blocks: CmsBlock[]
  canManage: boolean
  currentOgImage: string
  currentCanonical: string
  currentNofollow: boolean
  currentJsonld: string
  currentSpec: string
  onSaveSeo: (settings: SeoSettings) => Promise<unknown>
}) {
  const fid = useId()
  const [ogImage, setOgImage] = useState<ImageValue>(currentOgImage)
  const [noindex, setNoindex] = useState(seoNoindex)
  const [canonical, setCanonical] = useState(currentCanonical)
  const [nofollow, setNofollow] = useState(currentNofollow)
  const [jsonld, setJsonld] = useState(currentJsonld)
  const seededSpec = parseSpec(currentSpec)
  const [schemaMode, setSchemaMode] = useState<SchemaMode>(initialSchemaMode(currentJsonld, seededSpec))
  const [schemaImage, setSchemaImage] = useState(seededSpec?.image ?? false)
  const [schemaDatePublished, setSchemaDatePublished] = useState(seededSpec?.datePublished ?? false)
  const [schemaDateModified, setSchemaDateModified] = useState(seededSpec?.dateModified ?? false)
  const [schemaAuthor, setSchemaAuthor] = useState(seededSpec?.author ?? '')
  useEffect(() => {
    // Re-seed from the persisted values on page identity / save change
    /* eslint-disable react-hooks/set-state-in-effect */
    setOgImage(currentOgImage)
    setNoindex(seoNoindex)
    setCanonical(currentCanonical)
    setNofollow(currentNofollow)
    setJsonld(currentJsonld)
    const spec = parseSpec(currentSpec)
    setSchemaMode(initialSchemaMode(currentJsonld, spec))
    setSchemaImage(spec?.image ?? false)
    setSchemaDatePublished(spec?.datePublished ?? false)
    setSchemaDateModified(spec?.dateModified ?? false)
    setSchemaAuthor(spec?.author ?? '')
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [currentOgImage, seoNoindex, currentCanonical, currentNofollow, currentJsonld, currentSpec])

  const resolvedOg = assetPreviewUrl(ogImage)
  // Blank JSON-LD is fine; non-blank must parse or the storefront drops it
  const jsonldValid = useMemo(() => {
    const t = jsonld.trim()
    if (!t) return true
    try {
      JSON.parse(t)
      return true
    } catch {
      return false
    }
  }, [jsonld])

  // The spec the builder toggles describe, or null outside a generated mode - save stores
  // exactly one of raw JSON (custom) or the spec (generated), so they can't fight at render
  const builtSpec: SchemaSpec | null =
    schemaMode === 'WebPage' || schemaMode === 'Article'
      ? {
          type: schemaMode,
          ...(schemaImage ? { image: true } : {}),
          ...(schemaDatePublished ? { datePublished: true } : {}),
          ...(schemaDateModified ? { dateModified: true } : {}),
          ...(schemaMode === 'Article' && schemaAuthor.trim() ? { author: schemaAuthor.trim() } : {}),
        }
      : null
  const specForSave = builtSpec ? JSON.stringify(builtSpec) : ''
  const rawForSave = schemaMode === 'custom' ? jsonld.trim() : ''

  // "Edit as raw JSON" - freeze the generated output into the custom textarea so a power
  // user can hand-tweak it from there
  function editAsRaw() {
    const name = (metaTitle || title).trim() || slug
    const url = absUrl(canonical.trim() || '/' + urlPath.replace(/^\//, ''))
    const generated = buildSchemaFromSpec(builtSpec ?? { type: 'WebPage' }, {
      name,
      description: metaDescription.trim() || undefined,
      url,
      image: resolvedOg ? absUrl(resolvedOg) : undefined,
      publishedAt,
    })
    setJsonld(JSON.stringify(generated, null, 2))
    setSchemaMode('custom')
  }

  const filename = (u: string) => (u ? u.split('/').pop() ?? u : '')
  const diff: SectionDiffEntry[] = []
  if (resolvedOg !== currentOgImage) {
    diff.push({ label: 'OG image', before: filename(currentOgImage), after: filename(resolvedOg) })
  }
  if (noindex !== seoNoindex) {
    diff.push({ label: 'Search indexing', before: seoNoindex ? 'noindex' : 'index', after: noindex ? 'noindex' : 'index' })
  }
  if (canonical.trim() !== currentCanonical) {
    diff.push({ label: 'Canonical URL', before: currentCanonical, after: canonical.trim() })
  }
  if (nofollow !== currentNofollow) {
    diff.push({ label: 'Link following', before: currentNofollow ? 'nofollow' : 'follow', after: nofollow ? 'nofollow' : 'follow' })
  }
  const schemaAfterSig = rawForSave ? 'custom:' + rawForSave : specForSave ? 'spec:' + specForSave : ''
  const schemaBeforeSig = currentJsonld ? 'custom:' + currentJsonld : currentSpec ? 'spec:' + currentSpec : ''
  if (schemaAfterSig !== schemaBeforeSig) {
    diff.push({
      label: 'Structured data',
      before: schemaModeLabel(initialSchemaMode(currentJsonld, seededSpec)),
      after: schemaModeLabel(schemaMode),
    })
  }
  useSaveSection({
    id: 'seo',
    title: 'SEO',
    diff,
    save: async () => {
      await onSaveSeo({ ogImage: resolvedOg, noindex, canonical: canonical.trim(), nofollow, jsonld: rawForSave, schema: specForSave })
    },
  })

  const effectiveTitle = (metaTitle || title).trim()
  const fallbackImage = firstBlockImage(blocks)
  const socialImage = resolvedOg || fallbackImage
  const lint = runSeoLint({ title, metaTitle, metaDescription, slug, seoNoindex, blocks })

  return (
    <div className="grid gap-5 rounded-md border border-dashed p-3">
      <div className="grid gap-4 md:grid-cols-2">
        <div className="grid gap-1.5">
          <Label className="text-xs text-muted-foreground">Search result preview</Label>
          <SerpPreview title={effectiveTitle} description={metaDescription} urlPath={urlPath} />
        </div>
        <div className="grid gap-1.5">
          <Label className="text-xs text-muted-foreground">Social share preview</Label>
          <SocialPreview
            image={socialImage}
            title={effectiveTitle}
            description={metaDescription}
            usingFallback={!resolvedOg && !!fallbackImage}
          />
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label className="text-sm">Social share image (Open Graph)</Label>
        <MediaImageInput
          value={ogImage}
          onChange={(v) => setOgImage(v ?? '')}
          disabled={!canManage}
          path="cms/seo"
          placeholder="https://… or pick from the library"
        />
        <span className="text-xs text-muted-foreground">
          Used as the og:image when this page is shared. Falls back to the first page image if unset.
        </span>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <div className="flex items-center justify-between gap-3 rounded-md border p-3">
          <div className="grid gap-0.5">
            <Label htmlFor={`${fid}-allow-search-indexing`} className="text-sm">Allow search indexing</Label>
            <span className="text-xs text-muted-foreground">
              Off emits &lt;meta robots noindex&gt; - keeps this page out of search results.
            </span>
          </div>
          <Switch id={`${fid}-allow-search-indexing`} checked={!noindex} onCheckedChange={(v) => setNoindex(!v)} disabled={!canManage} />
        </div>
        <div className="flex items-center justify-between gap-3 rounded-md border p-3">
          <div className="grid gap-0.5">
            <Label htmlFor={`${fid}-follow-links`} className="text-sm">Follow links</Label>
            <span className="text-xs text-muted-foreground">
              Off adds nofollow - crawlers won&apos;t follow links from this page.
            </span>
          </div>
          <Switch id={`${fid}-follow-links`} checked={!nofollow} onCheckedChange={(v) => setNofollow(!v)} disabled={!canManage} />
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label className="text-sm" htmlFor="seo-canonical">Canonical URL</Label>
        <Input
          id="seo-canonical"
          value={canonical}
          onChange={(e) => setCanonical(e.target.value)}
          disabled={!canManage}
          placeholder="Leave blank to use this page's own URL"
        />
        <span className="text-xs text-muted-foreground">
          Points search engines at the primary URL for this content - use for duplicate or syndicated pages.
        </span>
      </div>

      <div className="grid gap-2">
        <Label htmlFor={`${fid}-structured-data-schema-o`} className="text-sm">Structured data (schema.org)</Label>
        <Select
          value={schemaMode}
          onValueChange={(v) => setSchemaMode(v as SchemaMode)}
          disabled={!canManage}
        >
          <SelectTrigger id={`${fid}-structured-data-schema-o`} className="h-9">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">None</SelectItem>
            <SelectItem value="WebPage">Web page (generated from this page)</SelectItem>
            <SelectItem value="Article">Article (generated from this page)</SelectItem>
            <SelectItem value="custom">Custom JSON-LD</SelectItem>
          </SelectContent>
        </Select>

        {(schemaMode === 'WebPage' || schemaMode === 'Article') && (
          <div className="grid gap-2 rounded-md border p-3">
            <span className="text-xs text-muted-foreground">
              Built fresh from the page - name, description and URL are always included, so a later edit flows through
            </span>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm">Image (from the OG image)</span>
              <Switch checked={schemaImage} onCheckedChange={setSchemaImage} disabled={!canManage} />
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm">Date published</span>
              <Switch checked={schemaDatePublished} onCheckedChange={setSchemaDatePublished} disabled={!canManage} />
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-sm">Date modified</span>
              <Switch checked={schemaDateModified} onCheckedChange={setSchemaDateModified} disabled={!canManage} />
            </div>
            {schemaMode === 'Article' && (
              <div className="grid gap-1">
                <Label className="text-xs" htmlFor="seo-author">Author</Label>
                <Input
                  id="seo-author"
                  value={schemaAuthor}
                  onChange={(e) => setSchemaAuthor(e.target.value)}
                  disabled={!canManage}
                  placeholder="Optional"
                />
              </div>
            )}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 justify-self-start"
              disabled={!canManage}
              onClick={editAsRaw}
            >
              Edit as raw JSON
            </Button>
          </div>
        )}

        {schemaMode === 'custom' && (
          <>
            <Textarea
              id="seo-jsonld"
              value={jsonld}
              onChange={(e) => setJsonld(e.target.value)}
              disabled={!canManage}
              rows={6}
              spellCheck={false}
              className="font-mono text-xs"
              placeholder={'{ "@context": "https://schema.org", "@type": "WebPage", "name": "…" }'}
            />
            {jsonld.trim() && !jsonldValid ? (
              <span className="text-xs text-destructive">
                Invalid JSON - fix it or the storefront will skip it
              </span>
            ) : (
              <span className="text-xs text-muted-foreground">
                Emitted as &lt;script type=&quot;application/ld+json&quot;&gt; for rich results
              </span>
            )}
          </>
        )}
      </div>

      <div className="grid gap-1.5">
        <Label className="text-sm">On-page checks</Label>
        <ul className="grid gap-1.5 rounded-md border p-2.5">
          {lint.map((r) => (
            <li key={r.id} className="flex items-start gap-2 text-sm">
              <LintIcon level={r.level} />
              <span>
                <span className="font-medium">{r.label}</span>{' '}
                <span className="text-muted-foreground">- {r.detail}</span>
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
