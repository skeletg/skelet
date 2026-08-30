import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { slugify } from '@/lib/slug'
import { useConfigScope } from '@/lib/config-scope'
import type { ReviewQuestionInputType } from '@/graphql/schema.types'
import {
  CreateReviewQuestionDocument,
  ReviewQuestionsDocument,
  UpdateReviewQuestionDocument,
  type ReviewQuestionFieldsFragment,
} from './Reviews.gql'

// Create + edit dialog for a single review question - On create the
// admin supplies a slug + input_type + the first locale's label - On
// edit only the runtime knobs (required, allow_comment, sort_order)
// are mutable - slug and input_type are immutable for the same reason
// catalog attribute slug + input_type are: changing them orphans
// existing answer rows - Translations land on the dedicated detail
// page (ReviewQuestionDetail), not here

const INPUT_TYPE_OPTIONS: {
  value: ReviewQuestionInputType
  label: string
  help: string
}[] = [
  { value: 'RATING_1_5', label: 'Rating 1–5', help: 'Star-style numeric rating with optional comment.' },
  { value: 'YES_NO', label: 'Yes / No', help: 'Boolean toggle with optional comment.' },
  { value: 'FREE_TEXT', label: 'Free text', help: 'Open-ended answer; no rating.' },
]

export function ReviewQuestionFormDialog({
  open,
  onOpenChange,
  attribute,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  // `attribute` is the existing question on edit, null on create Named to match AttributeFormDialog's prop so this
  // dialog drops into the same call sites if we ever consolidate
  attribute: ReviewQuestionFieldsFragment | null
}) {
  const { globalDefaultLocale } = useConfigScope()
  const isEdit = !!attribute

  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [label, setLabel] = useState('')
  const [helpText, setHelpText] = useState('')
  const [inputType, setInputType] = useState<ReviewQuestionInputType>('RATING_1_5')
  const [isRequired, setIsRequired] = useState(false)
  const [allowComment, setAllowComment] = useState(true)
  const [sortOrder, setSortOrder] = useState(0)
  const [error, setError] = useState<string | null>(null)

  // Hydrate/reset the form when the dialog opens or the edited question changes (state adjustment during render, keyed
  // on the open flag and the question itself)
  const [prevHydrateKey, setPrevHydrateKey] = useState<{
    open: boolean
    attribute: ReviewQuestionFieldsFragment | null
  } | null>(null)
  if (!prevHydrateKey || prevHydrateKey.open !== open || prevHydrateKey.attribute !== attribute) {
    setPrevHydrateKey({ open, attribute })
    if (open) {
      if (attribute) {
        setSlug(attribute.slug)
        setSlugTouched(true)
        setLabel(attribute.content?.label ?? '')
        setHelpText(attribute.content?.helpText ?? '')
        setInputType(attribute.inputType)
        setIsRequired(attribute.isRequired)
        setAllowComment(attribute.allowComment)
        setSortOrder(attribute.sortOrder)
      } else {
        setSlug('')
        setSlugTouched(false)
        setLabel('')
        setHelpText('')
        setInputType('RATING_1_5')
        setIsRequired(false)
        setAllowComment(true)
        setSortOrder(0)
      }
      setError(null)
    }
  }

  const [createQuestion, { loading: creating }] = useMutation(CreateReviewQuestionDocument, {
    refetchQueries: [{ query: ReviewQuestionsDocument }],
  })
  const [updateQuestion, { loading: updating }] = useMutation(UpdateReviewQuestionDocument, {
    refetchQueries: [{ query: ReviewQuestionsDocument }],
  })

  const saving = creating || updating
  const locale = globalDefaultLocale || 'en'

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      if (isEdit && attribute) {
        await updateQuestion({
          variables: {
            input: {
              id: attribute.id,
              isRequired,
              allowComment,
              sortOrder,
            },
          },
        })
      } else {
        if (!slug.trim() || !label.trim()) {
          setError('Slug and label are required.')
          return
        }
        await createQuestion({
          variables: {
            input: {
              slug: slug.trim(),
              inputType,
              isRequired,
              allowComment,
              sortOrder,
              locale,
              label: label.trim(),
              helpText: helpText.trim() || null,
            },
          },
        })
      }
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={onSubmit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle>{isEdit ? 'Edit question' : 'New review question'}</DialogTitle>
            <DialogDescription>
              {isEdit
                ? 'Slug and input type are immutable - changing them would orphan existing answer rows.'
                : 'Pick an input type and seed the first locale\'s prompt. Add translations later from the question detail page.'}
            </DialogDescription>
          </DialogHeader>

          {!isEdit ? (
            <>
              <div className="grid gap-2">
                <Label htmlFor="rq-label">Label ({locale})</Label>
                <Input
                  id="rq-label"
                  value={label}
                  onChange={(e) => {
                    setLabel(e.target.value)
                    if (!slugTouched) {
                      setSlug(slugify(e.target.value))
                    }
                  }}
                  placeholder="Grip on wet roads"
                  required
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="rq-slug">Slug</Label>
                <Input
                  id="rq-slug"
                  value={slug}
                  onChange={(e) => {
                    setSlug(e.target.value)
                    setSlugTouched(true)
                  }}
                  placeholder="wet_road_grip"
                  required
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="rq-help">Help text</Label>
                <Input
                  id="rq-help"
                  value={helpText}
                  onChange={(e) => setHelpText(e.target.value)}
                  placeholder="Optional - shown under the prompt on the channel"
                />
              </div>

              <div className="grid gap-2">
                <Label>Input type</Label>
                <Select
                  value={inputType}
                  onValueChange={(v) => setInputType(v as ReviewQuestionInputType)}
                >
                  <SelectTrigger>
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
            </>
          ) : null}

          <SwitchRow
            id="rq-required"
            label="Required"
            description="Customers can't submit a review without an answer."
            checked={isRequired}
            onCheckedChange={setIsRequired}
          />

          <SwitchRow
            id="rq-allow-comment"
            label="Allow comment"
            description="Show an optional text field next to the rating."
            checked={allowComment}
            onCheckedChange={setAllowComment}
          />

          <div className="grid gap-2">
            <Label htmlFor="rq-sort-order">Sort order</Label>
            <Input
              id="rq-sort-order"
              type="number"
              value={sortOrder}
              onChange={(e) => setSortOrder(Number(e.target.value))}
              className="max-w-[120px]"
            />
          </div>

          {error ? (
            <Alert variant="destructive">
              <AlertTitle>Couldn't save</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              Cancel
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SwitchRow({
  id,
  label,
  description,
  checked,
  onCheckedChange,
}: {
  id: string
  label: string
  description: string
  checked: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-4 rounded-lg border p-3">
      <div className="grid gap-0.5">
        <Label htmlFor={id} className="cursor-pointer text-sm font-medium">
          {label}
        </Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  )
}
