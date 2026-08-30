import { useCallback, useMemo, useState } from 'react'
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
  UpsertProductContentDocument,
} from '../Catalog.gql'
import { LexicalRichTextEditor, type RichTextImageUploadResult, type RichTextValue } from '../LexicalRichTextEditor'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'
import { Section, type ContentProduct } from './shared'
import { readImageDimensions } from './media-utils'
import { ProductMediaPanel } from './ProductMediaPanel'

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
  const content = product.content.find((entry) => entry.locale === locale)
  // Wire JSON -> editor boundary: the scalars arrive as unknown; the editor
  // narrows malformed values itself (isSerializedEditorState fallback)
  const originalShortDescription = (content?.shortDescription ?? product.shortDescription ?? null) as RichTextValue
  const originalDescriptionRichText = (content?.descriptionRichText ??
    product.descriptionRichText ??
    null) as RichTextValue
  const [shortDescription, setShortDescription] = useState<RichTextValue>(originalShortDescription)
  const [descriptionRichText, setDescriptionRichText] = useState<RichTextValue>(originalDescriptionRichText)
  const [mediaOpen, setMediaOpen] = useState(false)
  const existingName = content?.name ?? product.name ?? product.content[0]?.name ?? 'Untitled product'

  const refetchQueries = [
    'ProductsPaged',
    { query: ProductDocument, variables: productVariables },
  ]
  const [upsertContent] = useMutation(UpsertProductContentDocument, { refetchQueries })
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

  const shortDescriptionDirty = JSON.stringify(shortDescription) !== JSON.stringify(originalShortDescription)
  const descriptionDirty = JSON.stringify(descriptionRichText) !== JSON.stringify(originalDescriptionRichText)

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    if (shortDescriptionDirty) {
      entries.push({ label: 'Short description', before: 'previous', after: 'updated' })
    }
    if (descriptionDirty) {
      entries.push({ label: 'Description', before: 'previous', after: 'updated' })
    }
    return entries
  }, [shortDescriptionDirty, descriptionDirty])

  const save = useCallback(async () => {
    if (!shortDescriptionDirty && !descriptionDirty) return
    await upsertContent({
      variables: {
        productId: product.id,
        locale,
        input: {
          name: existingName,
          shortDescription: shortDescription ?? undefined,
          descriptionRichText: descriptionRichText ?? undefined,
        },
      },
    })
  }, [shortDescriptionDirty, descriptionDirty, shortDescription, descriptionRichText, existingName, product.id, locale, upsertContent])

  useProductSaveSection({ id: 'content', title: 'Content & media', diff, save })

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
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="pd-short-description">Short description</Label>
            <LexicalRichTextEditor
              value={shortDescription}
              fallbackText=""
              disabled={!canManage}
              placeholder="One-line teaser shown in listings"
              contentClassName="min-h-24"
              onUploadImage={canManage ? uploadShortDescriptionImage : undefined}
              onChange={setShortDescription}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="pd-description">Description</Label>
            <LexicalRichTextEditor
              value={descriptionRichText}
              fallbackText=""
              disabled={!canManage}
              onUploadImage={canManage ? uploadDescriptionImage : undefined}
              onChange={setDescriptionRichText}
            />
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
