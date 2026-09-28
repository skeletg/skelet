import { useMemo } from 'react'
import { Separator } from '@/components/ui/separator'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { cleanMetadata, type MetadataEntry } from '@/shared/schemas/metadata'
import { PRODUCT_METADATA_LABELS, productMetadataDiff, productMetadataSchema, toProductMetadataValues } from '../schemas/product'

// The product's public and private metadata - one staged section, written together in one update

export function ProductMetadataEditor({
  productId,
  metadata,
  privateMetadata,
  canManage,
  onSave,
}: {
  productId: string
  metadata: MetadataEntry[]
  privateMetadata: MetadataEntry[]
  canManage: boolean
  onSave: (input: { metadata: MetadataEntry[]; privateMetadata: MetadataEntry[] }) => Promise<unknown>
}) {
  const baseline = useMemo(() => toProductMetadataValues({ metadata, privateMetadata }), [metadata, privateMetadata])
  const { form, draft } = useSectionForm({
    id: 'metadata',
    title: 'Metadata',
    baseline,
    schema: productMetadataSchema,
    diff: productMetadataDiff,
    routeKey: `product-metadata:${productId}`,
    submit: async (v) => {
      await onSave({ metadata: cleanMetadata(v.metadata), privateMetadata: cleanMetadata(v.privateMetadata) })
    },
  })

  return (
    <div className="grid gap-6">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_METADATA_LABELS} />}
      <fieldset disabled={!canManage} className="grid gap-6">
        <section className="grid gap-3">
          <div className="grid gap-1">
            <h3 className="text-sm font-medium">Metadata</h3>
            <p className="text-sm text-muted-foreground">Publicly exposable key-value pairs for channels, APIs, and integrations.</p>
          </div>
          <form.AppField name="metadata">{(f) => <f.MetadataField />}</form.AppField>
        </section>
        <Separator />
        <section className="grid gap-3">
          <div className="grid gap-1">
            <h3 className="text-sm font-medium">Private metadata</h3>
            <p className="text-sm text-muted-foreground">Internal-only key-value pairs for admin workflows and backend integrations.</p>
          </div>
          <form.AppField name="privateMetadata">{(f) => <f.MetadataField label="Private metadata" />}</form.AppField>
        </section>
      </fieldset>
    </div>
  )
}
