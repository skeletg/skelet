import { useState, type ReactNode } from 'react'
import { ArchiveIcon, ChevronRightIcon, ClockIcon, Trash2Icon } from 'lucide-react'
import { TranslationLocales } from '@/components/TranslationLocales'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { FormRichTextEditor, type RichTextValue } from '@/features/catalog/LexicalRichTextEditor'
import type { BlogTagsQuery } from '../Blog.gql'
import type { PostValues } from '../schemas/post'
import type { FlatCategory } from './categories'
import { PostStores } from './PostStores'
import { statusLabel, statusVariant } from './status'
import type { Post, PostFormApi } from './types'

// Everything about a post but its words, docked beside the page. All but publishing and the stores' switches waits for Save
export function PostPanel({
  post,
  form,
  values,
  locale,
  canManage,
  canEditTranslations,
  categories,
  tags,
  busy,
  variables,
  noindexEdits,
  onNoindex,
  onSchedule,
  onArchive,
  onDelete,
  onRemoveTranslation,
  onSlugTyped,
}: {
  post: Post
  form: PostFormApi
  values: PostValues
  locale: string
  canManage: boolean
  canEditTranslations: boolean
  categories: FlatCategory[]
  tags: BlogTagsQuery['blogTags']
  busy: boolean
  variables: { id: string; includeUnpublished: boolean }
  noindexEdits: Map<string, boolean>
  onNoindex: (next: Map<string, boolean>) => void
  onSchedule: (at: string) => void
  onArchive: () => void
  onDelete: () => void
  onRemoveTranslation: () => Promise<void>
  onSlugTyped: () => void
}) {
  const [scheduleAt, setScheduleAt] = useState('')
  const localesWithContent = post.allContent.map((c) => c.locale)
  const toggle = (field: 'categoryIds' | 'tagIds', id: string) =>
    form.setFieldValue(field, (prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

  return (
    <aside aria-label="Post details" className="flex h-full w-72 shrink-0 flex-col border-l border-foreground/10 bg-background @min-[1200px]/editor:w-80">
      <h2 className="flex h-[49px] shrink-0 items-center border-b border-foreground/10 px-4 text-sm font-medium tracking-tight">Post details</h2>
      <div className="flex-1 overflow-y-auto">
        <PanelSection title="Publishing">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={statusVariant(post.status)}>{statusLabel(post.status)}</Badge>
            {post.publishedAt ? <span className="text-xs text-muted-foreground">{new Date(post.publishedAt).toLocaleString()}</span> : null}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="schedule">Schedule for</Label>
            <Input id="schedule" type="datetime-local" value={scheduleAt} onChange={(e) => setScheduleAt(e.target.value)} disabled={!canManage} />
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onSchedule(scheduleAt)}
              disabled={!canManage || busy || !scheduleAt}
            >
              <ClockIcon className="size-4" /> Schedule
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onArchive}
              disabled={!canManage || busy || post.status === 'ARCHIVED'}
            >
              <ArchiveIcon className="size-4" /> Archive
            </Button>
            {canManage ? (
              <Button type="button" variant="outline" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={onDelete}>
                <Trash2Icon className="size-4" />
                Delete
              </Button>
            ) : null}
          </div>
        </PanelSection>

        <fieldset disabled={!canManage} className="contents">
          <PanelSection title="Details">
            <form.AppField name="slug">
              {(f) => <f.TextField id="slug" label="Slug" hint="Changing it leaves a redirect from the old address." onValueChange={onSlugTyped} />}
            </form.AppField>
            <form.AppField name="readingTime">{(f) => <f.TextField id="reading-time" label="Reading time (min)" type="number" />}</form.AppField>
            {/* Which locales have content, and the remove action for the one being edited - the
                active locale is the header's; there is no in-form picker */}
            <TranslationLocales
              locale={canEditTranslations ? locale : ''}
              locales={localesWithContent}
              entityLabel="post"
              canManage={canManage}
              defaultLocaleLocked
              onRemove={onRemoveTranslation}
            />
          </PanelSection>

          <PanelSection title="Excerpt">
            <form.Field name="excerpt">
              {(f) => (
                <FormRichTextEditor
                  value={(f.state.value as RichTextValue) ?? undefined}
                  fallbackText=""
                  disabled={!canManage}
                  placeholder="Short summary (used in feeds and listings)…"
                  onChange={(v) => f.handleChange(v)}
                />
              )}
            </form.Field>
          </PanelSection>

          <PanelSection title="Categories and tags">
            <div className="grid gap-2">
              <Label>Categories</Label>
              {categories.length === 0 ? (
                <p className="text-sm text-muted-foreground">No categories yet.</p>
              ) : (
                <ul className="grid gap-1">
                  {categories.map((c) => (
                    <li key={c.id} className="flex items-center gap-2 rounded-md border px-2 py-1.5">
                      <Checkbox id={`cat-${c.id}`} checked={values.categoryIds.includes(c.id)} onCheckedChange={() => toggle('categoryIds', c.id)} />
                      <Label htmlFor={`cat-${c.id}`} className="min-w-0 flex-1 cursor-pointer truncate text-sm font-normal">
                        {c.label}
                      </Label>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <div className="grid gap-2">
              <Label>Tags</Label>
              {tags.length === 0 ? (
                <p className="text-sm text-muted-foreground">No tags yet.</p>
              ) : (
                <ul className="flex flex-wrap gap-2">
                  {tags.map((t) => {
                    const checked = values.tagIds.includes(t.id)
                    return (
                      <li key={t.id}>
                        <Button type="button" variant={checked ? 'secondary' : 'outline'} size="sm" aria-pressed={checked} onClick={() => toggle('tagIds', t.id)}>
                          #{t.content?.name ?? t.slug}
                        </Button>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          </PanelSection>

          <PanelSection title="Search engines">
            <div className="grid gap-0.5 rounded-md border bg-muted/40 p-3">
              <p className="truncate text-xs text-muted-foreground">/{values.slug || post.slug}</p>
              <p className="truncate text-sm font-medium text-info">{values.metaTitle || values.title || 'Post title'}</p>
              <p className="line-clamp-2 text-xs text-muted-foreground">{values.metaDescription || 'The description shown under the title in search results.'}</p>
            </div>
            <form.AppField name="metaTitle">
              {(f) => <f.TextField id="meta-title" label="Meta title" placeholder={values.title || 'Post title'} note={<LengthHint length={values.metaTitle.length} max={60} />} />}
            </form.AppField>
            <form.AppField name="metaDescription">
              {(f) => (
                <f.TextareaField
                  id="meta-description"
                  label="Meta description"
                  rows={3}
                  placeholder="Short search-result description…"
                  note={<LengthHint length={values.metaDescription.length} max={160} />}
                />
              )}
            </form.AppField>
            <form.Field name="seoNoindex">
              {(f) => (
                <div className="flex items-center gap-2">
                  <Switch id="seo-noindex" checked={f.state.value} onCheckedChange={f.handleChange} />
                  <Label htmlFor="seo-noindex">Hide from search engines (noindex)</Label>
                </div>
              )}
            </form.Field>
          </PanelSection>
        </fieldset>

        <PanelSection title="Stores">
          <PostStores postId={post.id} listings={post.channelListings} canManage={canManage} refetchVariables={variables} noindexEdits={noindexEdits} onNoindex={onNoindex} />
        </PanelSection>
      </div>
    </aside>
  )
}

// One part of the post's panel. It folds away but stays mounted, so what was typed in it is still there to save
function PanelSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <details open className="group border-b border-foreground/10 last:border-b-0">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-medium outline-none select-none hover:bg-muted/50 focus-visible:bg-muted/50 [&::-webkit-details-marker]:hidden">
        <ChevronRightIcon className="size-4 text-muted-foreground transition-transform group-open:rotate-90" />
        {title}
      </summary>
      <div className="grid gap-4 px-4 pt-1 pb-5">{children}</div>
    </details>
  )
}

// Search engines truncate past these lengths - a count beside the field, red once over
function LengthHint({ length, max }: { length: number; max: number }) {
  return <span className={`text-xs tabular-nums ${length > max ? 'text-destructive' : 'text-muted-foreground'}`}>{length}/{max}</span>
}
