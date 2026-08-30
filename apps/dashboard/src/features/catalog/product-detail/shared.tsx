import type { ReactNode } from 'react'
import {
  ChevronDownIcon,
  GlobeIcon,
  LanguagesIcon,
  StoreIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import type { ProductKind } from '@/graphql/schema.types'
import { SectionAnchor } from '@/components/layout/detail-page'
import { useTocSection } from '@/components/layout/toc'

export type ProductMediaSummary = {
  id: string
  url: string
  altText: string
  status: 'PENDING' | 'READY'
  sortOrder: number
  width: number
  height: number
  mimeType: string
  sizeBytes: number
}

export type ProductContentEntry = {
  name: string
  // Wire JSON scalars (Lexical documents) arrive as unknown; consumers assert to RichTextValue at the editor boundary,
  // where the editor's own isSerializedEditorState narrowing handles malformed values
  shortDescription?: unknown
  // description is server-side JSON (Lexical) and may be null in older rows;
  // ProductSeoEditor reads it via `content?.description ?? ''`
  description?: unknown
  descriptionRichText?: unknown
  locale: string
  metaDescription?: string | null
  metaKeywords?: string | null
  metaTitle?: string | null
}

export type ContentProduct = {
  id: string
  name?: string | null
  canonicalUrl?: string | null
  url: string
  urlAuto: boolean
  urlTemplateOverride: string
  metaDescription?: string | null
  metaTitle?: string | null
  seoNoindex: boolean
  shortDescription?: unknown
  description?: unknown
  descriptionRichText?: unknown
  content: ProductContentEntry[]
  mainImage?: ProductMediaSummary | null
  media: ProductMediaSummary[]
  ogImage?: ProductMediaSummary | null
  thumbnail?: ProductMediaSummary | null
  productType?: { id: string; slug: string; name: string; urlTemplate?: string } | null
}

export type ProductCategoryItem = { id: string; slug: string; urlPath: string; name?: string | null }

export function KindBadge({ kind }: { kind: ProductKind }) {
  const label = kind.charAt(0) + kind.slice(1).toLowerCase()
  const tone =
    kind === 'CONFIGURABLE'
      ? 'border-sky-200 bg-sky-50 text-sky-900 dark:border-sky-900/60 dark:bg-sky-950/40 dark:text-sky-200'
      : kind === 'SIMPLE'
        ? ''
        : kind === 'GROUPED'
          ? 'border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-200'
          : 'border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-200'
  return (
    <Badge variant={kind === 'SIMPLE' ? 'secondary' : 'outline'} className={`gap-1 ${tone}`}>
      {label}
    </Badge>
  )
}

// ---------- Layout helpers (categories-style flush sections) ----------

export function Section({
  title,
  description,
  trailing,
  children,
}: {
  title: ReactNode
  description?: ReactNode
  trailing?: ReactNode
  children: ReactNode
}) {
  const { ref, anchorId } = useTocSection(title)
  return (
    <section ref={ref} id={anchorId} className="grid gap-4 px-6 py-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="grid gap-1">
          <h3 className="text-lg font-semibold tracking-tight">
            <SectionAnchor id={anchorId} title={title} />{title}
          </h3>
          {description ? (
            <p className="text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {trailing}
      </div>
      <div className="grid gap-4">{children}</div>
    </section>
  )
}

// ---------- Scope indicator ----------

export function ScopeIndicator({
  mode,
  storeName,
  locale,
}: {
  mode: 'GLOBAL' | 'CHANNEL'
  storeName: string | null
  locale: string | null
}) {
  const isStore = mode === 'CHANNEL'
  return (
    <div
      className={`flex flex-wrap items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium shadow-xs ${
        isStore
          ? 'border-primary/40 bg-primary/5 text-foreground'
          : 'border-border bg-muted/40 text-muted-foreground'
      }`}
      title={isStore ? `${storeName ?? 'store'} (${locale ?? 'default'})` : 'Global / default locale'}
    >
      {isStore ? (
        <>
          <span className="inline-flex items-center gap-1.5">
            <StoreIcon className="size-3.5 text-primary" />
            <span className="font-semibold">{storeName ?? 'Selected store'}</span>
          </span>
          <span className="text-border">•</span>
          <span className="inline-flex items-center gap-1.5">
            <LanguagesIcon className="size-3.5 text-primary" />
            <code className="font-mono text-[11px] font-semibold">{locale ?? 'default'}</code>
          </span>
        </>
      ) : (
        <span className="inline-flex items-center gap-1.5">
          <GlobeIcon className="size-3.5" />
          <span className="font-semibold">Global</span>
          {locale ? (
            <>
              <span className="text-border">•</span>
              <code className="font-mono text-[11px] font-semibold">{locale}</code>
            </>
          ) : null}
        </span>
      )}
    </div>
  )
}

export function CollapsibleSection({
  title,
  description,
  open,
  onOpenChange,
  children,
}: {
  title: string
  description: ReactNode
  open: boolean
  onOpenChange: (open: boolean) => void
  children: ReactNode
}) {
  const { ref, anchorId } = useTocSection(title, { expand: () => onOpenChange(true) })
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} asChild>
      <section ref={ref} id={anchorId} className="grid gap-4 px-6 py-6">
        <CollapsibleTrigger asChild>
          <button className="group/collapsible flex w-full cursor-pointer items-start justify-between gap-4 text-left focus-visible:outline-none">
            <div className="grid gap-1">
              {/* Decorative here, unlike every other section: this heading is inside the collapsible's
                  trigger button, where a nested link is invalid markup and would fight the toggle -
                  the section still answers to its id, so deep links and the contents rail reach it */}
              <h3 className="text-lg font-semibold tracking-tight transition-colors group-hover/collapsible:text-primary group-focus-visible/collapsible:text-primary">
                <span className="mr-1.5 text-primary">#</span>{title}
              </h3>
              <p className="text-sm text-muted-foreground">{description}</p>
            </div>
            <ChevronDownIcon className={`mt-1 size-4 shrink-0 text-muted-foreground transition-all duration-[360ms] [transition-timing-function:cubic-bezier(0.33,1,0.68,1)] group-hover/collapsible:text-primary group-focus-visible/collapsible:text-primary ${open ? 'rotate-180' : ''}`} />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>{children}</CollapsibleContent>
      </section>
    </Collapsible>
  )
}
