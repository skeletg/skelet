import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PencilIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { changed } from '@/shared/saving/diff'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope } from '@/lib/config-scope'
import {
  DeleteReviewQuestionDocument,
  ReviewQuestionDocument,
  ReviewQuestionsDocument,
  UpsertReviewQuestionContentDocument,
} from './Reviews.gql'
import { ReviewQuestionFormDialog } from './ReviewQuestionFormDialog'

// Per-question editor - Header carries the slug + type + required/comment badges and the Edit/Delete actions; body
// sections cover settings recap and translations - Mirrors the ProductDetail template layout

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
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('reviews.manage')
  const { availableLocales, globalDefaultLocale } = useConfigScope()

  const { data, loading, error, refetch } = useQuery(ReviewQuestionDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const question = data?.reviewQuestion

  const [editingSettings, setEditingSettings] = useState(false)

  // Every translation row stages into this registry and the ONE header Save applies them together -
  // a per-row Save button is what the convention forbids
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveAllError, setSaveAllError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveAllError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveAllError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const [deleteQuestion, { loading: deleting }] = useMutation(DeleteReviewQuestionDocument, {
    refetchQueries: [{ query: ReviewQuestionsDocument }],
  })

  async function onDelete() {
    if (!question) return
    setDeleteError(null)
    try {
      await deleteQuestion({ variables: { id: question.id } })
      navigate('/reviews/questions')
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (loading) return <LoadingState />
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

  const localesNeedingTranslation = availableLocales.filter(
    (code) => !question.allContent.some((c) => c.locale === code),
  )

  const displayName = question.content?.label || question.slug

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
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
              {question.allContent.length}{' '}
              {question.allContent.length === 1 ? 'translation' : 'translations'}
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
                onClick={() => {
                  setDeleteError(null)
                  setConfirmingDelete(true)
                }}
                disabled={deleting}
              >
                <Trash2Icon className="size-4" />
                Delete
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!isDirty || savingAll}
                onClick={() => setConfirmingSave(true)}
              >
                {savingAll ? 'Saving…' : 'Save changes'}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => setEditingSettings(true)}
              >
                <PencilIcon className="size-4" />
                Edit settings
              </Button>
            </>
          ) : null
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Translations"
          description="Per-locale label and help text shown on the channel review form. Missing locales fall back to the slug."
        >
          <div className="grid gap-4">
            {question.allContent.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No translations yet. The channel will fall back to the
                slug as the prompt. Add at least one locale below.
              </p>
            ) : (
              question.allContent.map((c) => (
                <TranslationRow
                  key={c.locale}
                  questionId={question.id}
                  locale={c.locale}
                  initialLabel={c.label}
                  initialHelpText={c.helpText}
                  canManage={canManage}
                />
              ))
            )}
            {canManage && localesNeedingTranslation.length > 0 ? (
              <div className="mt-2 grid gap-2">
                <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Add a translation
                </p>
                {localesNeedingTranslation.slice(0, 3).map((code) => (
                  <TranslationRow
                    key={`new-${code}`}
                    questionId={question.id}
                    locale={code}
                    initialLabel=""
                    initialHelpText=""
                    canManage={canManage}
                    emphasize={code === globalDefaultLocale}
                  />
                ))}
              </div>
            ) : null}
          </div>
        </DetailPageSection>
      </DetailPageBody>

      <ReviewQuestionFormDialog
        open={editingSettings}
        onOpenChange={setEditingSettings}
        attribute={question}
      />

      <AlertDialog open={confirmingDelete} onOpenChange={setConfirmingDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this question?</AlertDialogTitle>
            <AlertDialogDescription>
              Removes the question from every product type and erases every
              existing answer row. Past reviews keep their headers.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError ? (
            <Alert variant="destructive">
              <AlertTitle>Couldn't delete</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault()
                void onDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Review question · ${question.slug}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveAllError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveAllError(null) } }}
        onConfirm={onConfirmSave}
      />
    </SaveRegistryContext.Provider>
  )
}

function TranslationRow({
  questionId,
  locale,
  initialLabel,
  initialHelpText,
  canManage,
  emphasize,
}: {
  questionId: string
  locale: string
  initialLabel: string
  initialHelpText: string
  canManage: boolean
  emphasize?: boolean
}) {
  const [label, setLabel] = useState(initialLabel)
  const [helpText, setHelpText] = useState(initialHelpText)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect -- intentional: resync the form fields when a different question/locale loads into this row */
    setLabel(initialLabel)
    setHelpText(initialHelpText)
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [initialLabel, initialHelpText])

  const [upsert] = useMutation(UpsertReviewQuestionContentDocument, {
    refetchQueries: [
      { query: ReviewQuestionDocument, variables: { id: questionId } },
      { query: ReviewQuestionsDocument },
    ],
  })

  const dirty = label !== initialLabel || helpText !== initialHelpText
  const exists = initialLabel !== ''

  // Registered with the page: the diff is what the header dialog lists, and `save` is a call-time
  // thunk reading live state, so a clipped label can never persist a stale draft
  useSaveSection({
    id: `content:${locale}`,
    title: `Translation · ${locale}`,
    diff: [
      ...(label !== initialLabel ? [changed(`Label (${locale})`, initialLabel, label)] : []),
      ...(helpText !== initialHelpText
        ? [changed(`Help text (${locale})`, initialHelpText, helpText)]
        : []),
    ].filter((d): d is NonNullable<typeof d> => d !== null),
    save: () => onSave(),
  })

  async function onSave() {
    setError(null)
    try {
      await upsert({
        variables: {
          input: {
            questionId,
            locale,
            label: label.trim(),
            helpText: helpText.trim() || null,
          },
        },
      })
      setSavedAt(Date.now())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
      throw err // the header dialog keeps itself open and surfaces the message
    }
  }

  return (
    <div className={`grid gap-2 rounded-lg border p-3 ${emphasize ? 'border-primary/40 bg-primary/5' : ''}`}>
      <div className="flex items-center justify-between gap-2">
        <Label className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
          {locale}
          {exists ? null : <span className="ml-2 text-[10px] text-amber-600">missing</span>}
        </Label>
        {/* No per-row Save - the page header applies every staged row through one dialog */}
        {dirty ? (
          <span className="text-[10px] text-amber-600">unsaved</span>
        ) : exists ? null : (
          <span className="text-[10px] text-muted-foreground">not translated</span>
        )}
      </div>
      <Input
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        placeholder="Question label"
        disabled={!canManage}
      />
      <Input
        value={helpText}
        onChange={(e) => setHelpText(e.target.value)}
        placeholder="Optional help text"
        disabled={!canManage}
      />
      {savedAt && !dirty ? (
        <p className="text-[10px] text-muted-foreground">Saved.</p>
      ) : null}
      {error ? (
        <p className="text-[10px] text-destructive">{error}</p>
      ) : null}
    </div>
  )
}
