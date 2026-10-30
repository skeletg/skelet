import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { SparklesIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { GenerateCmsPageSeoDocument, type CmsPageRowFragment } from '../Cms.gql'
import { SeoPanel, type SeoSettings } from '../SeoPanel'
import type { CmsBlock } from '../page-blocks'
import type { ContentForm } from './ContentCard'

// The locale's meta title and description, then the page-level SEO settings. Meta copy is per locale and
// rides in the content form; the settings are page-wide and save as their own section
export function SeoCard({
  page,
  locale,
  form,
  canManage,
  canEditTranslations,
  aiEnabled,
  onSaveSeo,
}: {
  page: CmsPageRowFragment
  locale: string
  form: ContentForm
  canManage: boolean
  canEditTranslations: boolean
  aiEnabled: boolean
  // Persists the page-level SEO settings (OG image, indexing, canonical, link-following, JSON-LD) in one
  // write - they share the metadata map, so a per-field save would clobber the others
  onSaveSeo: (settings: SeoSettings) => Promise<unknown>
}) {
  const values = useStore(form.store, (s) => s.values)
  const editable = canManage && canEditTranslations
  const seoMeta = (k: string) => page.metadata?.find((m) => m.key === k)?.value ?? ''
  const [genSeo, { loading: genningSeo }] = useMutation(GenerateCmsPageSeoDocument)
  const [aiErr, setAiErr] = useState<string | null>(null)

  async function onGenerateSeo() {
    setAiErr(null)
    try {
      const res = await genSeo({ variables: { pageId: page.id, locale } })
      const s = res.data?.generateCMSPageSeo
      if (s?.metaTitle) form.setFieldValue('metaTitle', s.metaTitle)
      if (s?.metaDescription) form.setFieldValue('metaDescription', s.metaDescription)
    } catch (e) {
      setAiErr(e instanceof Error ? e.message : 'AI generation failed.')
    }
  }

  return (
    <div className="grid gap-4">
      {aiErr && (
        <Alert variant="destructive">
          <AlertTitle>AI</AlertTitle>
          <AlertDescription>{aiErr}</AlertDescription>
        </Alert>
      )}
      <fieldset disabled={!editable} className="grid gap-4">
        {aiEnabled && editable && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-8 justify-self-start"
            disabled={genningSeo}
            onClick={onGenerateSeo}
            title="Generate an SEO meta title + description from this page's content"
          >
            <SparklesIcon className="size-4" />
            {genningSeo ? 'Generating…' : 'Generate with AI'}
          </Button>
        )}
        <form.AppField name="metaTitle">{(f) => <f.TextField id="meta-title" label="Meta title" />}</form.AppField>
        <form.AppField name="metaDescription">{(f) => <f.TextareaField id="meta-description" label="Meta description" rows={2} />}</form.AppField>
      </fieldset>
      <SeoPanel
        title={values.title}
        metaTitle={values.metaTitle}
        metaDescription={values.metaDescription}
        slug={page.slug}
        urlPath={page.urlPath}
        seoNoindex={page.seoNoindex}
        publishedAt={page.publishedAt ?? null}
        blocks={values.blocks as CmsBlock[]}
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
