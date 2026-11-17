import type { ReactNode } from 'react'
import { useStore, type AnyFormApi } from '@tanstack/react-form'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { hasDetailSections } from '@/plugins/detail-section-registry'
import { PluginAside, PluginSections } from '@/plugins/detail-sections'
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
  plugins,
  readOnly = false,
  hideSave = false,
}: {
  record: RecordFormFrame
  /** Null leaves the back link out - a top-level page has nowhere to go back to */
  backTo: string | null
  backLabel?: string
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
  /** Below the form body and outside it - sections whose own buttons mustn't touch the form */
  after?: ReactNode
  /** The record plugins' sections render for - above the body, after `after`, and in a side panel */
  plugins?: { target: string; entity: unknown; mode: 'edit' | 'create' }
  /** No write permission - every field disabled and the Save button left out */
  readOnly?: boolean
  /** The page renders its own submit buttons in `actions` - a create with two ways to save, say */
  hideSave?: boolean
}) {
  const { form, draft, registry, confirm, submitError, save, isEdit, label } = record
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  // Side sections render at the end of the body too, where the panel is hidden on a narrow screen
  const pluginsAfter =
    !!plugins && (hasDetailSections(plugins.target, plugins.mode, 'after') || hasDetailSections(plugins.target, plugins.mode, 'side'))

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout rightSidebar={plugins && <PluginAside {...plugins} />}>
        <DetailPageHeader
          backTo={backTo}
          backLabel={backLabel}
          title={title}
          meta={meta}
          badges={badges}
          actions={
            <>
              {actions}
              {!readOnly && !hideSave && (
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

        {plugins && hasDetailSections(plugins.target, plugins.mode, 'before') && (
          <DetailPageBody className="border-b pb-0">
            <PluginSections {...plugins} placement="before" />
          </DetailPageBody>
        )}

        <DetailPageBody>
          {draft.offer !== null && (
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

        {(after || pluginsAfter) && (
          <DetailPageBody className="border-t-0">
            {after}
            {plugins && <PluginSections {...plugins} />}
          </DetailPageBody>
        )}

        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={label} sections={registry.dirtySections} />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}
