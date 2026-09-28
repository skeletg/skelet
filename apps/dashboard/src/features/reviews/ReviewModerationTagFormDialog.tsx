import { useMutation } from '@apollo/client/react'
import { ReviewLabelFormDialog, type ReviewLabelCopy } from './ReviewLabelFormDialog'
import {
  CreateReviewModerationTagDocument,
  ReviewModerationTagsDocument,
  UpdateReviewModerationTagDocument,
  type ReviewModerationTagFieldsFragment,
} from './Reviews.gql'

const COPY: ReviewLabelCopy = {
  idPrefix: 'rmt',
  editTitle: 'Edit moderation tag',
  newTitle: 'New moderation tag',
  editDescription: 'The slug is immutable - it keys reviews already flagged with this tag.',
  newDescription: 'A moderation concern the AI can flag on a review (and a moderator can apply by hand).',
  namePlaceholder: 'Profanity',
  slugPlaceholder: 'profanity',
  descriptionPlaceholder:
    'Tells the AI when to flag a review with this tag, e.g. “Contains curse words, profanity, or vulgar language.”',
  descriptionHint: 'Written for the classifier - the clearer this is, the more accurately it flags.',
  activeHint:
    "Only active tags are offered to the AI. Inactive ones keep their past assignments but won't be flagged on new reviews.",
}

// Create + edit dialog for one moderation tag - see ReviewLabelFormDialog
export function ReviewModerationTagFormDialog({
  open,
  onOpenChange,
  tag,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  tag: ReviewModerationTagFieldsFragment | null
}) {
  const refetchQueries = [{ query: ReviewModerationTagsDocument }]
  const [createTag] = useMutation(CreateReviewModerationTagDocument, { refetchQueries })
  const [updateTag] = useMutation(UpdateReviewModerationTagDocument, { refetchQueries })
  return (
    <ReviewLabelFormDialog
      open={open}
      onOpenChange={onOpenChange}
      label={tag}
      copy={COPY}
      writes={{
        create: (input) => createTag({ variables: { input } }),
        update: (input) => updateTag({ variables: { input } }),
      }}
    />
  )
}
