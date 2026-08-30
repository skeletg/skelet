import { useEffect, useState } from 'react'
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
import { Textarea } from '@/components/ui/textarea'
import { slugify } from '@/lib/slug'
import {
  CreateReviewCategoryDocument,
  ReviewCategoriesDocument,
  UpdateReviewCategoryDocument,
  type ReviewCategoryFieldsFragment,
} from './Reviews.gql'

// Create + edit dialog for a single review category - The slug is immutable
// after creation (it's the stable key the AI returns and reviews FK to), like
// review question slugs - The DESCRIPTION is the important field: it's the
// instruction the classifier reads to decide whether a review belongs here, so
// the placeholder nudges the operator to write it for the model
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
  const isEdit = !!category

  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [isActive, setIsActive] = useState(true)
  const [sortOrder, setSortOrder] = useState(0)
  const [error, setError] = useState<string | null>(null)

  // Seed the form fields from the category prop when the dialog opens (reset on create) - Matches ReviewQuestionFormDialog; the disable is
  // the same idiom ReviewDetail uses for syncing local draft state from a server prop
  useEffect(() => {
    if (!open) return
    if (category) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- intentional: see comment above.
      setSlug(category.slug)
      setSlugTouched(true)
      setName(category.name)
      setDescription(category.description)
      setIsActive(category.isActive)
      setSortOrder(category.sortOrder)
    } else {
      setSlug('')
      setSlugTouched(false)
      setName('')
      setDescription('')
      setIsActive(true)
      setSortOrder(0)
    }
    setError(null)
  }, [open, category])

  const [createCategory, { loading: creating }] = useMutation(CreateReviewCategoryDocument, {
    refetchQueries: [{ query: ReviewCategoriesDocument }],
  })
  const [updateCategory, { loading: updating }] = useMutation(UpdateReviewCategoryDocument, {
    refetchQueries: [{ query: ReviewCategoriesDocument }],
  })

  const saving = creating || updating

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name.trim()) {
      setError('Name is required.')
      return
    }
    try {
      if (isEdit && category) {
        await updateCategory({
          variables: {
            input: {
              id: category.id,
              name: name.trim(),
              description: description.trim() || null,
              isActive,
              sortOrder,
            },
          },
        })
      } else {
        if (!slug.trim()) {
          setError('Slug is required.')
          return
        }
        await createCategory({
          variables: {
            input: {
              slug: slug.trim(),
              name: name.trim(),
              description: description.trim() || null,
              isActive,
              sortOrder,
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
            <DialogTitle>{isEdit ? 'Edit category' : 'New review category'}</DialogTitle>
            <DialogDescription>
              {isEdit
                ? 'The slug is immutable - it keys existing reviews and the AI verdict.'
                : 'A topical group the AI sorts each review into during moderation.'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-2">
            <Label htmlFor="rc-name">Name</Label>
            <Input
              id="rc-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                if (!isEdit && !slugTouched) setSlug(slugify(e.target.value))
              }}
              placeholder="Late shipment"
              required
            />
          </div>

          {!isEdit ? (
            <div className="grid gap-2">
              <Label htmlFor="rc-slug">Slug</Label>
              <Input
                id="rc-slug"
                value={slug}
                onChange={(e) => {
                  setSlug(e.target.value)
                  setSlugTouched(true)
                }}
                placeholder="late_shipment"
                required
              />
            </div>
          ) : null}

          <div className="grid gap-2">
            <Label htmlFor="rc-description">Description</Label>
            <Textarea
              id="rc-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Tells the AI when a review belongs here, e.g. “Customer reports the order arrived later than promised, slow delivery, or a carrier delay.”"
              rows={3}
            />
            <p className="text-xs text-muted-foreground">
              Written for the classifier - the clearer this is, the more accurately reviews land in
              this group.
            </p>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-lg border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="rc-active" className="cursor-pointer text-sm font-medium">
                Active
              </Label>
              <p className="text-xs text-muted-foreground">
                Only active categories are offered to the AI. Inactive ones keep their past
                assignments but won't be picked for new reviews.
              </p>
            </div>
            <Switch id="rc-active" checked={isActive} onCheckedChange={setIsActive} />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="rc-sort-order">Sort order</Label>
            <Input
              id="rc-sort-order"
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
