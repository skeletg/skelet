import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useConfigScope } from '@/lib/config-scope'
import { slugify } from '@/lib/slug'
import { CreateReviewQuestionDocument, ReviewQuestionsDocument, UpdateReviewQuestionDocument, type ReviewQuestionFieldsFragment } from './Reviews.gql'
import {
  createReviewQuestionSchema,
  editReviewQuestionSchema,
  toReviewQuestionCreateInput,
  toReviewQuestionUpdateInput,
  toReviewQuestionValues,
  type INPUT_TYPES,
} from './schemas/reviewQuestion'

const INPUT_TYPE_OPTIONS: { value: (typeof INPUT_TYPES)[number]; label: string; help: string }[] = [
  { value: 'RATING_1_5', label: 'Rating 1–5', help: 'Star-style numeric rating with optional comment.' },
  { value: 'YES_NO', label: 'Yes / No', help: 'Boolean toggle with optional comment.' },
  { value: 'FREE_TEXT', label: 'Free text', help: 'Open-ended answer; no rating.' },
]

// Create + edit dialog for one review question - see schemas/reviewQuestion for what each mode can change
export function ReviewQuestionFormDialog({
  open,
  onOpenChange,
  attribute,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The existing question on edit, null on create - named to match AttributeFormDialog's prop
  attribute: ReviewQuestionFieldsFragment | null
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange}>
      {(close) => <ReviewQuestionForm question={attribute} onClose={close} />}
    </FormDialog>
  )
}

function ReviewQuestionForm({ question, onClose }: { question: ReviewQuestionFieldsFragment | null; onClose: () => void }) {
  const locale = useConfigScope().globalDefaultLocale || 'en'
  const [slugTouched, setSlugTouched] = useState(false)
  const refetchQueries = [{ query: ReviewQuestionsDocument }]
  const [createQuestion] = useMutation(CreateReviewQuestionDocument, { refetchQueries })
  const [updateQuestion] = useMutation(UpdateReviewQuestionDocument, { refetchQueries })

  const dialog = useDialogForm({
    baseline: toReviewQuestionValues(question),
    schema: question ? editReviewQuestionSchema : createReviewQuestionSchema,
    submit: async (v) => {
      if (question) await updateQuestion({ variables: { input: toReviewQuestionUpdateInput(v, question.id) } })
      else await createQuestion({ variables: { input: toReviewQuestionCreateInput(v, locale) } })
    },
    onSaved: onClose,
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title={question ? 'Edit question' : 'New review question'}
      description={
        question
          ? 'Slug and input type are immutable - changing them would orphan existing answer rows.'
          : "Pick an input type and seed the first locale's prompt. Add translations later from the question detail page."
      }
      submitLabel={question ? 'Save changes' : 'Create'}
      onCancel={onClose}
    >
      {!question && (
        <>
          <form.AppField
            name="label"
            listeners={{
              onChange: ({ value }) => {
                if (!slugTouched) form.setFieldValue('slug', slugify(value))
              },
            }}
          >
            {(f) => <f.TextField id="rq-label" label={`Label (${locale})`} placeholder="Grip on wet roads" />}
          </form.AppField>
          <form.AppField name="slug">
            {(f) => (
              <f.TextField id="rq-slug" label="Slug" placeholder="wet_road_grip" onValueChange={() => setSlugTouched(true)} />
            )}
          </form.AppField>
          <form.AppField name="helpText">
            {(f) => (
              <f.TextField id="rq-help" label="Help text" placeholder="Optional - shown under the prompt on the channel" />
            )}
          </form.AppField>
          <form.Field name="inputType">
            {(f) => (
              <div className="grid gap-2">
                <Label htmlFor="rq-input-type">Input type</Label>
                <Select value={f.state.value} onValueChange={(v) => f.handleChange(v as (typeof INPUT_TYPES)[number])}>
                  <SelectTrigger id="rq-input-type" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {INPUT_TYPE_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        <div className="flex flex-col">
                          <span>{opt.label}</span>
                          <span className="text-[10px] text-muted-foreground">{opt.help}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </form.Field>
        </>
      )}

      <form.AppField name="isRequired">
        {(f) => (
          <f.SwitchField id="rq-required" label="Required" description="Customers can't submit a review without an answer." />
        )}
      </form.AppField>
      <form.AppField name="allowComment">
        {(f) => (
          <f.SwitchField id="rq-allow-comment" label="Allow comment" description="Show an optional text field next to the rating." />
        )}
      </form.AppField>
      <form.AppField name="sortOrder">
        {(f) => <f.NumberField id="rq-sort-order" label="Sort order" className="max-w-30" />}
      </form.AppField>
    </FormDialogBody>
  )
}
