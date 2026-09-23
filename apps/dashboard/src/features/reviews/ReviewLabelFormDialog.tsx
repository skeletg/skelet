import { useState } from 'react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { slugify } from '@/lib/slug'
import {
  reviewLabelSchema,
  toReviewLabelInput,
  toReviewLabelValues,
  type ExistingReviewLabel,
  type ReviewLabelValues,
} from './schemas/reviewLabel'

// The one create + edit dialog behind review categories and moderation tags - the two differ only
// in their wording and their mutations

export type ReviewLabelCopy = {
  /** Field id prefix - 'rc', 'rmt' */
  idPrefix: string
  editTitle: string
  newTitle: string
  editDescription: string
  newDescription: string
  namePlaceholder: string
  slugPlaceholder: string
  descriptionPlaceholder: string
  descriptionHint: string
  activeHint: string
}

export type ReviewLabelWrites = {
  create: (input: ReturnType<typeof toReviewLabelInput> & { slug: string }) => Promise<unknown>
  update: (input: ReturnType<typeof toReviewLabelInput> & { id: string }) => Promise<unknown>
}

export function ReviewLabelFormDialog({
  open,
  onOpenChange,
  label,
  copy,
  writes,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  label: ExistingReviewLabel | null
  copy: ReviewLabelCopy
  writes: ReviewLabelWrites
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange}>
      {(close) => <ReviewLabelForm label={label} copy={copy} writes={writes} onClose={close} />}
    </FormDialog>
  )
}

function ReviewLabelForm({
  label,
  copy,
  writes,
  onClose,
}: {
  label: ExistingReviewLabel | null
  copy: ReviewLabelCopy
  writes: ReviewLabelWrites
  onClose: () => void
}) {
  const [slugTouched, setSlugTouched] = useState(!!label)
  const dialog = useDialogForm({
    baseline: toReviewLabelValues(label),
    schema: reviewLabelSchema,
    submit: async (v: ReviewLabelValues) => {
      if (label) await writes.update({ id: label.id, ...toReviewLabelInput(v) })
      else await writes.create({ slug: v.slug.trim(), ...toReviewLabelInput(v) })
    },
    onSaved: onClose,
  })
  const { form } = dialog
  const p = copy.idPrefix

  return (
    <FormDialogBody
      dialog={dialog}
      title={label ? copy.editTitle : copy.newTitle}
      description={label ? copy.editDescription : copy.newDescription}
      submitLabel={label ? 'Save changes' : 'Create'}
      onCancel={onClose}
    >
      <form.AppField
        name="name"
        listeners={{
          onChange: ({ value }) => {
            if (!label && !slugTouched) form.setFieldValue('slug', slugify(value))
          },
        }}
      >
        {(f) => <f.TextField id={`${p}-name`} label="Name" placeholder={copy.namePlaceholder} />}
      </form.AppField>

      {!label && (
        <form.AppField name="slug">
          {(f) => (
            <f.TextField
              id={`${p}-slug`}
              label="Slug"
              placeholder={copy.slugPlaceholder}
              onValueChange={() => setSlugTouched(true)}
            />
          )}
        </form.AppField>
      )}

      <form.AppField name="description">
        {(f) => (
          <f.TextareaField
            id={`${p}-description`}
            label="Description"
            rows={3}
            placeholder={copy.descriptionPlaceholder}
            hint={copy.descriptionHint}
          />
        )}
      </form.AppField>

      <form.AppField name="isActive">
        {(f) => <f.SwitchField id={`${p}-active`} label="Active" description={copy.activeHint} />}
      </form.AppField>

      <form.AppField name="sortOrder">
        {(f) => <f.NumberField id={`${p}-sort-order`} label="Sort order" className="max-w-[120px]" />}
      </form.AppField>
    </FormDialogBody>
  )
}
