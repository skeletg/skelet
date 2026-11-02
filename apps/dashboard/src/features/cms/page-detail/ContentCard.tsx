import { useState } from 'react'
import { useStore } from '@tanstack/react-form'
import { LanguagesIcon, SparklesIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import type { PersistedForm } from '@/shared/saving/usePersistedForm'
import type { useSectionForm } from '@/shared/saving/useSectionForm'
import { type CmsPageRowFragment } from '../Cms.gql'
import { DynamicField, type FieldValue } from '../DynamicFieldRenderer'
import { PAGE_CONTENT_LABELS, type PageContentValues } from '../schemas/page'
import { type PageTypeAttr } from './shared'

export type ContentForm = ReturnType<typeof useSectionForm<PageContentValues>>['form']

// The locale's title and translatable fields. The form is the page's (CmsPageDetail) - the block layout
// in it is shared with the builder section and the live preview
export function ContentCard({
  page,
  locale,
  form,
  draft,
  canManage,
  canEditTranslations,
  translatableAttrs,
  aiEnabled,
  onTranslate,
}: {
  page: CmsPageRowFragment
  locale: string
  form: ContentForm
  draft: PersistedForm
  canManage: boolean
  canEditTranslations: boolean
  translatableAttrs: PageTypeAttr[]
  // Whether the AI assistant is usable (gates the Generate/Translate buttons)
  aiEnabled: boolean
  // Translate this locale's content from another locale (parent owns the mutation + refetch)
  onTranslate: (sourceLocale: string) => Promise<unknown>
}) {
  const data = useStore(form.store, (s) => s.values.data) as Record<string, FieldValue>
  const editable = canManage && canEditTranslations

  // AI assistant - suggestion-only: it fills the fields, and the header Save persists them
  const [translating, setTranslating] = useState(false)
  const [aiErr, setAiErr] = useState<string | null>(null)
  // Other locales that have content to translate FROM
  const translateSources = page.allContent.filter((c) => c.locale !== locale && (c.title || c.body)).map((c) => c.locale)
  const [translateSource, setTranslateSource] = useState('')

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

  return (
    <div className="grid gap-4">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PAGE_CONTENT_LABELS} />}

      {aiEnabled && editable && translateSources.length > 0 && (
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
          <Button type="button" size="sm" variant="secondary" className="h-8" disabled={translating} onClick={onRunTranslate}>
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

      <fieldset disabled={!editable} className="grid gap-4">
        <form.AppField name="title">{(f) => <f.TextField id="title" label="Title" />}</form.AppField>
        {translatableAttrs.map((pta) => (
          <DynamicField
            key={pta.attribute.id}
            attribute={pta.attribute}
            isRequired={pta.isRequired || pta.attribute.isRequiredDefault}
            disabled={!editable}
            value={data[pta.attribute.slug]}
            onChange={(v) => form.setFieldValue('data', (prev) => ({ ...prev, [pta.attribute.slug]: v }))}
          />
        ))}
      </fieldset>
    </div>
  )
}
