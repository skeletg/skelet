import { useMutation } from '@apollo/client/react'
import { ReviewLabelFormDialog, type ReviewLabelCopy } from './ReviewLabelFormDialog'
import {
  CreateReviewCategoryDocument,
  ReviewCategoriesDocument,
  UpdateReviewCategoryDocument,
  type ReviewCategoryFieldsFragment,
} from './Reviews.gql'

const COPY: ReviewLabelCopy = {
  idPrefix: 'rc',
  editTitle: 'Edit category',
  newTitle: 'New review category',
  editDescription: 'The slug is immutable - it keys existing reviews and the AI verdict.',
  newDescription: 'A topical group the AI sorts each review into during moderation.',
  namePlaceholder: 'Late shipment',
  slugPlaceholder: 'late_shipment',
  descriptionPlaceholder:
    'Tells the AI when a review belongs here, e.g. “Customer reports the order arrived later than promised, slow delivery, or a carrier delay.”',
  descriptionHint: 'Written for the classifier - the clearer this is, the more accurately reviews land in this group.',
  activeHint:
    "Only active categories are offered to the AI. Inactive ones keep their past assignments but won't be picked for new reviews.",
}

// Create + edit dialog for one review category - see ReviewLabelFormDialog
export function ReviewCategoryFormDialog({
  open,
  onOpenChange,
  category,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The existing category on edit, null on create
  category: ReviewCategoryFieldsFragment | null
}) {
  const refetchQueries = [{ query: ReviewCategoriesDocument }]
  const [createCategory] = useMutation(CreateReviewCategoryDocument, { refetchQueries })
  const [updateCategory] = useMutation(UpdateReviewCategoryDocument, { refetchQueries })
  return (
    <ReviewLabelFormDialog
      open={open}
      onOpenChange={onOpenChange}
      label={category}
      copy={COPY}
      writes={{
        create: (input) => createCategory({ variables: { input } }),
        update: (input) => updateCategory({ variables: { input } }),
      }}
    />
  )
}
