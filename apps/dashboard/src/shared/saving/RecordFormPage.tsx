import type { ReactNode } from 'react'
import { useStore, type AnyFormApi } from '@tanstack/react-form'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { SaveConfirmDialog } from './SaveConfirmDialog'
import { SaveRegistryContext, type SaveRegistryValue, type SectionRegistration } from './saveRegistry'
import { RestoredDraftBanner } from './RestoredDraftBanner'
import type { PersistedForm } from './usePersistedForm'

// The frame around a useRecordForm page - header with its one Save, the restored-draft banner, the
// create-failure alert, the body, and the confirm dialog - so the page itself is just its sections

type RecordFormFrame = {
  form: AnyFormApi
  draft: PersistedForm
  registry: { contextValue: SaveRegistryValue; dirtySections: SectionRegistration[]; isDirty: boolean }
  confirm: { dialogProps: { open: boolean; saving: boolean; error: string | null; onConfirm: () => void; onCancel: () => void } }
  submitError: string | null
  save: () => void
  isEdit: boolean
  label: string
}

export function RecordFormPage({
  record,
  backTo,
  backLabel,
  title,
  meta,
  badges,
  createLabel,
  actions,
  labels,
  children,
  after,
  readOnly = false,
}: {
  record: RecordFormFrame
  backTo: string
  backLabel: string
  title: ReactNode
  meta?: ReactNode
  badges?: ReactNode
  /** The header button while creating - 'Create country' */
  createLabel: string
  /** Extra header actions left of Save - a Cancel link, say */
  actions?: ReactNode
  /** Field name -> label, for the restored-draft clash list */
  labels?: Record<string, string>
  children: ReactNode
  /** Below the form body and outside it - plugin sections, whose own buttons mustn't touch the form */
  after?: ReactNode
  /** No write permission - every field disabled and the Save button left out */
  readOnly?: boolean
}) {
  const { form, draft, registry, confirm, submitError, save, isEdit, label } = record
  const submitting = useStore(form.store, (s) => s.isSubmitting)

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <DetailPageHeader
          backTo={backTo}
          backLabel={backLabel}
          title={title}
          meta={meta}
          badges={badges}
          actions={
            <>
              {actions}
              {!readOnly && (
                <Button
                  type="button"
                  size="sm"
                  disabled={submitting || (isEdit && !registry.isDirty)}
                  onClick={save}
                >
                  {submitting ? 'Saving…' : isEdit ? 'Save changes' : createLabel}
                </Button>
              )}
            </>
          }
        />

        <DetailPageBody>
          {draft.restoredAt !== null && (
            <div className="px-6 py-4">
              <RestoredDraftBanner draft={draft} labels={labels} />
            </div>
          )}
          {submitError && (
            <div className="px-6 py-4">
              <Alert variant="destructive">
                <AlertTitle>{isEdit ? 'Update failed' : 'Create failed'}</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          )}
          {/* display:contents keeps the section layout while one attribute disables every control */}
          <fieldset disabled={readOnly} className="contents">
            {children}
          </fieldset>
        </DetailPageBody>

        {after && <DetailPageBody className="border-t-0">{after}</DetailPageBody>}

        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={label} sections={registry.dirtySections} />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}
