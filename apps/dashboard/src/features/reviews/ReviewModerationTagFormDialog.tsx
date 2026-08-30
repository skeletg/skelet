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
  CreateReviewModerationTagDocument,
  ReviewModerationTagsDocument,
  UpdateReviewModerationTagDocument,
  type ReviewModerationTagFieldsFragment,
} from './Reviews.gql'

// Create + edit dialog for a single moderation tag - Mirrors the category dialog:
// slug is immutable after creation (it's the stable key the AI returns and is
// stored on each review), and the DESCRIPTION is what the classifier reads to
// decide whether to flag a review with this concern
export function ReviewModerationTagFormDialog({
  open,
  onOpenChange,
  tag,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  tag: ReviewModerationTagFieldsFragment | null
}) {
  const isEdit = !!tag

  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [isActive, setIsActive] = useState(true)
  const [sortOrder, setSortOrder] = useState(0)
  const [error, setError] = useState<string | null>(null)

  // Seed the form fields from the tag prop when the dialog opens (reset on
  // create) - Same idiom ReviewCategoryFormDialog / ReviewDetail use
  useEffect(() => {
    if (!open) return
    if (tag) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- intentional: see comment above.
      setSlug(tag.slug)
      setSlugTouched(true)
      setName(tag.name)
      setDescription(tag.description)
      setIsActive(tag.isActive)
      setSortOrder(tag.sortOrder)
    } else {
      setSlug('')
      setSlugTouched(false)
      setName('')
      setDescription('')
      setIsActive(true)
      setSortOrder(0)
    }
    setError(null)
  }, [open, tag])

  const [createTag, { loading: creating }] = useMutation(CreateReviewModerationTagDocument, {
    refetchQueries: [{ query: ReviewModerationTagsDocument }],
  })
  const [updateTag, { loading: updating }] = useMutation(UpdateReviewModerationTagDocument, {
    refetchQueries: [{ query: ReviewModerationTagsDocument }],
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
      if (isEdit && tag) {
        await updateTag({
          variables: {
            input: {
              id: tag.id,
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
        await createTag({
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
            <DialogTitle>{isEdit ? 'Edit moderation tag' : 'New moderation tag'}</DialogTitle>
            <DialogDescription>
              {isEdit
                ? 'The slug is immutable - it keys reviews already flagged with this tag.'
                : 'A moderation concern the AI can flag on a review (and a moderator can apply by hand).'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-2">
            <Label htmlFor="rmt-name">Name</Label>
            <Input
              id="rmt-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                if (!isEdit && !slugTouched) setSlug(slugify(e.target.value))
              }}
              placeholder="Profanity"
              required
            />
          </div>

          {!isEdit ? (
            <div className="grid gap-2">
              <Label htmlFor="rmt-slug">Slug</Label>
              <Input
                id="rmt-slug"
                value={slug}
                onChange={(e) => {
                  setSlug(e.target.value)
                  setSlugTouched(true)
                }}
                placeholder="profanity"
                required
              />
            </div>
          ) : null}

          <div className="grid gap-2">
            <Label htmlFor="rmt-description">Description</Label>
            <Textarea
              id="rmt-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Tells the AI when to flag a review with this tag, e.g. “Contains curse words, profanity, or vulgar language.”"
              rows={3}
            />
            <p className="text-xs text-muted-foreground">
              Written for the classifier - the clearer this is, the more accurately it flags.
            </p>
          </div>

          <div className="flex items-start justify-between gap-4 rounded-lg border p-3">
            <div className="grid gap-0.5">
              <Label htmlFor="rmt-active" className="cursor-pointer text-sm font-medium">
                Active
              </Label>
              <p className="text-xs text-muted-foreground">
                Only active tags are offered to the AI. Inactive ones keep their past assignments but
                won't be flagged on new reviews.
              </p>
            </div>
            <Switch id="rmt-active" checked={isActive} onCheckedChange={setIsActive} />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="rmt-sort-order">Sort order</Label>
            <Input
              id="rmt-sort-order"
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
