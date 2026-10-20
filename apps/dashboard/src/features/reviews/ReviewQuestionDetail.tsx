import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { PencilIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext } from '@/shared/saving/saveRegistry'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope } from '@/lib/config-scope'
import {
  DeleteReviewQuestionDocument,
  ReviewQuestionDocument,
  ReviewQuestionsDocument,
  UpsertReviewQuestionContentDocument,
  type ReviewQuestionFieldsFragment,
} from './Reviews.gql'
import { ReviewQuestionFormDialog } from './ReviewQuestionFormDialog'
import {
  QUESTION_TRANSLATIONS_LABELS,
  changedTranslations,
  questionTranslationsDiff,
  questionTranslationsSchema,
  toQuestionTranslationsValues,
  toTranslationInput,
} from './schemas/reviewQuestion'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

// Per-question editor - Header carries the slug + type + required/comment badges and the Edit/Delete actions; body
// sections cover settings recap and translations - Mirrors the ProductDetail template layout

// Missing locales offered as blank rows to fill in - a handful, not the whole list
const NEW_LOCALE_ROWS = 3

// Standalone route wrapper: gives ReviewQuestionDetail the page scroll container it needs when reached directly at
// /reviews/questions/:id (outside the Reviews shell, which otherwise provides the layout)
export function ReviewQuestionDetailPage() {
  return (
    <DetailPageLayout>
      <ReviewQuestionDetail />
    </DetailPageLayout>
  )
}

export function ReviewQuestionDetail({ questionId }: { questionId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = questionId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery(ReviewQuestionDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const question = data?.reviewQuestion

  if (loading && !question) return <LoadingState />
  if (error) {
    return (
      <LoadError
        title="Couldn't load question"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  }
  if (!question) {
    return (
      <Alert>
        <AlertTitle>Question not found</AlertTitle>
      </Alert>
    )
  }
  // Keyed on the question - picking another one in the list starts a fresh form
  return <QuestionEditor key={question.id} question={question} />
}

// Every translation row is one form - the ONE header Save reviews and writes them together; a
// per-row Save button is what the convention forbids
function QuestionEditor({ question }: { question: ReviewQuestionFieldsFragment }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('reviews.manage')
  const { availableLocales, globalDefaultLocale } = useConfigScope()

  const [editingSettings, setEditingSettings] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const [deleteQuestion, { loading: deleting }] = useMutation(DeleteReviewQuestionDocument, {
    refetchQueries: [ReviewQuestionsDocument],
  })
  const [upsert] = useMutation(UpsertReviewQuestionContentDocument, {
    refetchQueries: [ReviewQuestionsDocument],
  })

  // The locale list comes back as a fresh array each render - key the offered rows on its content
  const newLocalesKey = availableLocales
    .filter((code) => !question.allContent.some((c) => c.locale === code))
    .slice(0, NEW_LOCALE_ROWS)
    .join(',')
  const baseline = useMemo(
    () => toQuestionTranslationsValues(question, newLocalesKey ? newLocalesKey.split(',') : []),
    [question, newLocalesKey],
  )

  const record = useRecordForm({
    id: 'translations',
    title: 'Translations',
    label: `Review question · ${question.slug}`,
    isEdit: true,
    baseline,
    schema: questionTranslationsSchema,
    diff: questionTranslationsDiff,
    persist: { routeKey: `review-question:${question.id}` },
    submit: async (v) => {
      for (const row of changedTranslations(baseline, v)) {
        await upsert({ variables: { input: toTranslationInput(question.id, row) } })
      }
    },
  })
  const { form, registry, confirm, draft } = record
  const submitting = useStore(form.store, (s) => s.isSubmitting)

  async function onDelete() {
    await deleteQuestion({ variables: { id: question.id } })
    navigate('/reviews/questions')
  }

  const displayName = question.content?.label || question.slug
  const existingCount = question.allContent.length

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageHeader
        backTo="/reviews/questions"
        backLabel="Back to questions"
        title={displayName}
        badges={
          <>
            <Badge variant="outline" className="font-mono text-[0.65rem] uppercase">
              {question.inputType.toLowerCase().replace('_', ' ')}
            </Badge>
            {question.isRequired ? <Badge variant="secondary">Required</Badge> : null}
            {question.allowComment ? <Badge variant="secondary">Comments</Badge> : null}
          </>
        }
        meta={
          <>
            <code className="font-mono text-foreground">{question.slug}</code>
            <span>·</span>
            <span>Sort {question.sortOrder}</span>
            <span>·</span>
            <span>
              {existingCount} {existingCount === 1 ? 'translation' : 'translations'}
            </span>
          </>
        }
        actions={
          canManage ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => setConfirmingDelete(true)}
                disabled={deleting}
              >
                <Trash2Icon className="size-4" />
                Delete
              </Button>
              <Button type="button" size="sm" variant="outline" disabled={!registry.isDirty || submitting} onClick={confirm.open}>
                {submitting ? 'Saving…' : 'Save changes'}
              </Button>
              <Button type="button" size="sm" onClick={() => setEditingSettings(true)}>
                <PencilIcon className="size-4" />
                Edit settings
              </Button>
            </>
          ) : null
        }
      />

      <DetailPageBody>
        {draft.offer !== null && (
          <div className="px-6 py-4">
            <RestoredDraftBanner draft={draft} labels={QUESTION_TRANSLATIONS_LABELS} />
          </div>
        )}
        <DetailPageSection
          title="Translations"
          description="Per-locale label and help text shown on the channel review form. Missing locales fall back to the slug."
        >
          <fieldset disabled={!canManage} className="grid gap-4">
            {existingCount === 0 && (
              <p className="text-sm text-muted-foreground">
                No translations yet. The channel will fall back to the slug as the prompt. Add at least one locale below.
              </p>
            )}
            <form.Field name="rows" mode="array">
              {(rows) =>
                rows.state.value.map((row, i) => {
                  const stored = baseline.rows.find((r) => r.locale === row.locale)
                  const dirty = !stored || stored.label !== row.label || stored.helpText !== row.helpText
                  return (
                    <div key={row.locale} className="grid gap-2">
                      {i === existingCount && canManage && (
                        <p className="mt-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Add a translation</p>
                      )}
                      <div
                        className={`grid gap-2 rounded-lg border p-3 ${!row.exists && row.locale === globalDefaultLocale ? 'border-foreground/30 bg-muted/40' : ''}`}
                      >
                        <div className="flex items-center justify-between gap-2">
                          <Label className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
                            {row.locale}
                            {row.exists ? null : <span className="ml-2 text-[10px] text-warning">missing</span>}
                          </Label>
                          {dirty ? (
                            <span className="text-[10px] text-warning">unsaved</span>
                          ) : row.exists ? null : (
                            <span className="text-[10px] text-muted-foreground">not translated</span>
                          )}
                        </div>
                        <form.AppField name={`rows[${i}].label`}>
                          {(f) => <f.TextField aria-label={`Label (${row.locale})`} placeholder="Question label" />}
                        </form.AppField>
                        <form.AppField name={`rows[${i}].helpText`}>
                          {(f) => <f.TextField aria-label={`Help text (${row.locale})`} placeholder="Optional help text" />}
                        </form.AppField>
                      </div>
                    </div>
                  )
                })
              }
            </form.Field>
          </fieldset>
        </DetailPageSection>
      </DetailPageBody>

      <ReviewQuestionFormDialog open={editingSettings} onOpenChange={setEditingSettings} attribute={question} />

      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title="Delete this question?"
        description="Removes the question from every product type and erases every existing answer row. Past reviews keep their headers."
        confirmLabel="Delete"
        busyLabel="Deleting…"
        destructive
        onConfirm={onDelete}
      />
      <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={record.label} sections={registry.dirtySections} />
    </SaveRegistryContext.Provider>
  )
}
