import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useStore } from '@tanstack/react-form'
import { CheckCircle2Icon, CircleOffIcon, Trash2Icon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { TranslationLocales } from '@/components/TranslationLocales'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useEditingLocale } from '@/lib/editing-locale'
import { slugify } from '@/lib/slug'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import {
  TERM_LABELS,
  currentTermName,
  termDiff,
  termSchema,
  toTermValues,
  type ExistingTerm,
  type TermValues,
} from './schemas/term'

// The one editor behind blog categories and tags - same record, same page - so the two route files
// are only their queries and mutations

export type TermKind = {
  /** 'category' / 'tag' - also the draft key and the delete wording */
  noun: string
  title: (name: string, slug: string) => string
  backTo: string
  nameRequired: boolean
  defaultPublished: boolean
  namePlaceholder: string
  slugPlaceholder: string
  deleteDescription: string
}

export type TermWrites = {
  create: (values: { slug: string; name: string; isPublished: boolean; locale: string }) => Promise<unknown>
  update: (id: string, values: { slug: string | null; isPublished: boolean }) => Promise<unknown>
  /** The per-locale name */
  upsertName: (id: string, locale: string, name: string) => Promise<unknown>
  remove: (id: string) => Promise<unknown>
  removeTranslation: (id: string, locale: string) => Promise<unknown>
}

export function BlogTermForm({ kind, term, writes }: { kind: TermKind; term?: ExistingTerm; writes: TermWrites }) {
  const isNew = !term
  const navigate = useNavigate()
  const canManage = usePermissions().has('blog.manage')
  const locale = useEditingLocale()
  // UI state, not the admin's work - an existing slug is never re-derived from the name
  const [slugTouched, setSlugTouched] = useState(!isNew)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const baseline = useMemo(() => toTermValues(term, kind.defaultPublished), [term, kind.defaultPublished])
  const schema = useMemo(() => termSchema({ nameRequired: kind.nameRequired }), [kind.nameRequired])

  const record = useRecordForm({
    id: kind.noun,
    title: kind.noun[0].toUpperCase() + kind.noun.slice(1),
    label: `${kind.title(baseline.name, baseline.slug)} · ${locale}`,
    isEdit: !isNew,
    baseline,
    schema,
    diff: termDiff,
    // The name is per locale, so a draft typed in one locale mustn't come back in another
    persist: { routeKey: `blog-${kind.noun}:${term?.id ?? 'new'}:${locale}` },
    submit: async (v: TermValues) => {
      const slug = v.slug.trim()
      // An emptied tag name falls back to the slug, as it always has
      const name = v.name.trim() || slug
      if (!term) {
        await writes.create({ slug, name, isPublished: v.isPublished, locale })
        return
      }
      await writes.update(term.id, { slug: slug !== term.slug ? slug : null, isPublished: v.isPublished })
      if (name !== currentTermName(term)) await writes.upsertName(term.id, locale, name)
    },
    onSaved: () => navigate(kind.backTo),
  })
  const { form, draft } = record
  const values = useStore(form.store, (s) => s.values)

  return (
    <>
      <RecordFormPage
        record={record}
        readOnly={!canManage}
        backTo={kind.backTo}
        backLabel="Back to taxonomy"
        title={isNew ? `New ${kind.noun}` : kind.title(values.name, values.slug)}
        badges={
          isNew ? null : values.isPublished ? (
            <Badge variant="secondary" className="gap-1">
              <CheckCircle2Icon className="size-3" /> Published
            </Badge>
          ) : (
            <Badge variant="outline" className="gap-1 text-muted-foreground">
              <CircleOffIcon className="size-3" /> Draft
            </Badge>
          )
        }
        createLabel={`Create ${kind.noun}`}
        actions={
          <>
            {term && canManage && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2Icon className="size-4" />
                Delete
              </Button>
            )}
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to={kind.backTo}>Cancel</Link>
            </Button>
          </>
        }
        labels={TERM_LABELS}
      >
        <DetailPageSection title="Basics">
          <div className="grid max-w-2xl gap-4">
            <form.AppField
              name="name"
              listeners={{
                // New terms derive the slug from the name until the admin edits it themselves
                onChange: ({ value }) => {
                  if (isNew && !slugTouched) form.setFieldValue('slug', slugify(value))
                },
              }}
            >
              {(f) => (
                <f.TextField
                  id="name"
                  label={`Name (${locale})`}
                  placeholder={kind.namePlaceholder}
                  autoFocus={isNew}
                  note={<ClashNote draft={draft} field="name" />}
                />
              )}
            </form.AppField>
            <form.AppField name="slug">
              {(f) => (
                <f.TextField
                  id="slug"
                  label="Slug"
                  placeholder={kind.slugPlaceholder}
                  onValueChange={() => setSlugTouched(true)}
                  hint={isNew && !slugTouched && values.name ? 'Generated from the name - type here to override.' : undefined}
                  note={<ClashNote draft={draft} field="slug" />}
                />
              )}
            </form.AppField>
            <form.Field name="isPublished">
              {(f) => (
                <div className="flex items-center gap-2">
                  <Switch id="published" checked={f.state.value} onCheckedChange={f.handleChange} />
                  <Label htmlFor="published">Published</Label>
                </div>
              )}
            </form.Field>
            {term && (
              <TranslationLocales
                locale={locale}
                locales={term.allContent.map((c) => c.locale)}
                entityLabel={kind.noun}
                canManage={canManage}
                onRemove={async () => {
                  await writes.removeTranslation(term.id, locale)
                }}
              />
            )}
          </div>
        </DetailPageSection>
      </RecordFormPage>

      {term && (
        <ConfirmDialog
          open={confirmDelete}
          onOpenChange={setConfirmDelete}
          title={`Delete this ${kind.noun}?`}
          description={kind.deleteDescription}
          confirmLabel="Delete"
          busyLabel="Deleting…"
          destructive
          onConfirm={async () => {
            await writes.remove(term.id)
            navigate(kind.backTo, { replace: true })
          }}
        />
      )}
    </>
  )
}
