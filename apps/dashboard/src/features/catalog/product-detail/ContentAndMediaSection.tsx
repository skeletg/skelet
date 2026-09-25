import { useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { ChevronDownIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import type { ProductMediaUploadTarget } from '@/graphql/schema.types'
import {
  ConfirmProductMediaUploadDocument,
  ProductDocument,
  RequestProductMediaUploadDocument,
} from '../Catalog.gql'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { FormRichTextEditor, type RichTextImageUploadResult, type RichTextValue } from '../LexicalRichTextEditor'
import {
  PRODUCT_CONTENT_LABELS,
  productContentChanges,
  productContentDiff,
  productContentSchema,
  toProductContentValues,
} from '../schemas/product'
import { Section, type ContentProduct } from './shared'
import { readImageDimensions } from './media-utils'
import { ProductMediaPanel } from './ProductMediaPanel'
import { useProductContentWrite } from './productContent'

const DESCRIPTION_UPLOAD_TARGET: ProductMediaUploadTarget = 'DESCRIPTION'
const SHORT_DESCRIPTION_UPLOAD_TARGET: ProductMediaUploadTarget = 'SHORT_DESCRIPTION'

export function ContentAndMediaSection({
  product,
  locale,
  scopeLabel,
  canManage,
  productVariables,
}: {
  product: ContentProduct
  locale: string
  scopeLabel: string
  canManage: boolean
  productVariables: { id: string }
}) {
  const [mediaOpen, setMediaOpen] = useState(false)

  const refetchQueries = [
    'ProductsPaged',
    { query: ProductDocument, variables: productVariables },
  ]
  const writeContent = useProductContentWrite(product.id, locale)
  const [requestUpload] = useMutation(RequestProductMediaUploadDocument)
  const [confirmUpload] = useMutation(ConfirmProductMediaUploadDocument, { refetchQueries })

  async function uploadRichTextImage(
    file: File,
    target: ProductMediaUploadTarget,
  ): Promise<RichTextImageUploadResult | null> {
    if (!file.type.startsWith('image/')) {
      throw new Error('Only image files can be uploaded here.')
    }

    const altText = window.prompt('Image alt text (optional)', '')?.trim()
    if (altText == null) return null

    const [{ width, height }, upload] = await Promise.all([
      readImageDimensions(file),
      requestUpload({
        variables: {
          input: {
            productId: product.id,
            filename: file.name,
            mimeType: file.type,
            sizeBytes: file.size,
            altText,
            target,
          },
        },
      }),
    ])
    const payload = upload.data?.requestProductMediaUpload
    if (!payload) throw new Error('Upload request did not return a signed URL.')

    const uploaded = await fetch(payload.uploadUrl, {
      method: 'PUT',
      headers: { 'Content-Type': file.type },
      body: file,
    })
    if (!uploaded.ok) throw new Error(`Image upload failed with HTTP ${uploaded.status}.`)

    const confirmed = await confirmUpload({
      variables: {
        mediaId: payload.media.id,
        width,
        height,
      },
    })
    const media = confirmed.data?.confirmProductMediaUpload
    if (!media) throw new Error('Image upload confirmation failed.')

    return {
      altText: media.altText,
      height: media.height,
      src: media.url,
      width: media.width,
    }
  }

  async function uploadDescriptionImage(file: File) {
    return uploadRichTextImage(file, DESCRIPTION_UPLOAD_TARGET)
  }

  async function uploadShortDescriptionImage(file: File) {
    return uploadRichTextImage(file, SHORT_DESCRIPTION_UPLOAD_TARGET)
  }

  // Wire JSON -> editor boundary: the scalars arrive as unknown; the editor narrows malformed values
  // itself (isSerializedEditorState fallback)
  const baseline = useMemo(() => toProductContentValues(product, locale), [product, locale])
  const { form, draft } = useSectionForm({
    id: 'content',
    title: 'Content & media',
    baseline,
    schema: productContentSchema,
    diff: productContentDiff,
    routeKey: `product-content:${product.id}:${locale}`,
    submit: async (v) => {
      await writeContent(productContentChanges(baseline, v))
    },
  })

  return (
    <Section
      title="Content & media"
      description={`Localized public copy for ${scopeLabel}. Use the short description for listings and previews, the full description for the product page body, and media roles to control product imagery across channel surfaces.`}
      trailing={
        <Badge variant="outline" className="font-mono font-normal">
          {locale}
        </Badge>
      }
    >
      <div className="grid gap-6">
        {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_CONTENT_LABELS} />}
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label>Short description</Label>
            <form.Field name="shortDescription">
              {(f) => (
                <FormRichTextEditor
                  value={f.state.value as RichTextValue}
                  fallbackText=""
                  disabled={!canManage}
                  placeholder="One-line teaser shown in listings"
                  contentClassName="min-h-24"
                  onUploadImage={canManage ? uploadShortDescriptionImage : undefined}
                  onChange={(next, plainText) => {
                    f.handleChange(next)
                    form.setFieldValue('shortDescriptionText', plainText)
                  }}
                />
              )}
            </form.Field>
          </div>

          <div className="grid gap-2">
            <Label>Description</Label>
            <form.Field name="descriptionRichText">
              {(f) => (
                <FormRichTextEditor
                  value={f.state.value as RichTextValue}
                  fallbackText=""
                  disabled={!canManage}
                  onUploadImage={canManage ? uploadDescriptionImage : undefined}
                  onChange={(next, plainText) => {
                    f.handleChange(next)
                    form.setFieldValue('descriptionText', plainText)
                  }}
                />
              )}
            </form.Field>
          </div>
        </div>

        <Separator />

        <Collapsible open={mediaOpen} onOpenChange={setMediaOpen}>
          <CollapsibleTrigger asChild>
            <button className="flex w-full items-start justify-between gap-4 rounded-md border bg-muted/40 px-4 py-3 text-left transition-colors hover:bg-muted/70">
              <div className="grid gap-1">
                <span className="text-base font-medium">Media</span>
                <span className="text-sm text-muted-foreground">
                  Upload product imagery and assign main, thumbnail, and OG image roles.
                </span>
              </div>
              <ChevronDownIcon className={`mt-1 size-4 shrink-0 text-muted-foreground transition-transform ${mediaOpen ? 'rotate-180' : ''}`} />
            </button>
          </CollapsibleTrigger>
          <CollapsibleContent className="pt-4">
            <div className="grid gap-3">
              <p className="text-sm text-muted-foreground">
                The main image is used on product pages, the thumbnail is used in compact listings,
                and the OG image is used for social previews.
              </p>
              <ProductMediaPanel
                productId={product.id}
                media={product.media}
                roles={{
                  MAIN: product.mainImage ?? null,
                  THUMBNAIL: product.thumbnail ?? null,
                  OG_IMAGE: product.ogImage ?? null,
                }}
                canManage={canManage}
                productVariables={productVariables}
              />
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </Section>
  )
}
