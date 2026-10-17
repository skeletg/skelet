import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { usePermissions } from '@/shared/auth/usePermissions'
import { SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { SchedulerConfigDocument, type SchedulerConfigQuery } from './Scheduler.gql'
import { DEFAULT_SCOPE } from './scheduler-config'
import {
  PATH_ENABLED,
  changedSchedulerConfig,
  schedulerSettingsSchema,
  toSchedulerSettingsValues,
} from './schemas/schedulerSettings'

/**
 * SchedulerSettings is the global config strip: master pause switch, failure alert webhook, and
 * run-history retention. It sits on the jobs list, which has no header Save, so the strip carries
 * its own - the pause switch writes the moment it's flipped, being a lifecycle action rather than a
 * field. Writes refetch the shared SchedulerConfig query so the paused banner stays in sync
 */
export function SchedulerSettings() {
  const { has } = usePermissions()
  const canView = has('config.view')
  const { data } = useQuery(SchedulerConfigDocument, {
    variables: { scope: DEFAULT_SCOPE },
    skip: !canView,
  })

  // Global settings are config-backed; hide the strip entirely from admins without config
  // visibility (they keep per-job scheduler controls)
  if (!canView || !data) return null
  return <SchedulerSettingsForm config={data} canEdit={has('config.manage_defaults')} />
}

function SchedulerSettingsForm({ config, canEdit }: { config: SchedulerConfigQuery; canEdit: boolean }) {
  const [setConfigValue, { loading: pausing }] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: SchedulerConfigDocument, variables: { scope: DEFAULT_SCOPE } }],
  })
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  const enabled = config.enabled?.value !== 'false'
  const alertIsSet = config.alertWebhookUrl?.isSet ?? false
  async function setEnabled(on: boolean) {
    setError(null)
    try {
      await setConfigValue({ variables: { path: PATH_ENABLED, value: on ? 'true' : 'false', scope: DEFAULT_SCOPE } })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  const baseline = useMemo(() => toSchedulerSettingsValues(config), [config])
  const form = useAppForm({
    defaultValues: baseline,
    validators: { onChange: schedulerSettingsSchema },
    onSubmit: async ({ value }) => {
      setError(null)
      try {
        for (const { path, value: v } of changedSchedulerConfig(baseline, value)) {
          await setConfigValue({ variables: { path, value: v, scope: DEFAULT_SCOPE } })
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Save failed.')
        return
      }
      form.reset(value)
      setSaved(true)
    },
  })
  const values = useStore(form.store, (s) => s.values)
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  const dirty = changedSchedulerConfig(baseline, values).length > 0
  useUnsavedChangesGuard(dirty, 'Scheduler settings')

  return (
    <form
      className="rounded-md border p-4"
      onSubmit={(e) => {
        e.preventDefault()
        e.stopPropagation()
        void form.handleSubmit()
      }}
    >
      <div className="mb-3 flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">Scheduler settings</h2>
          <p className="text-xs text-muted-foreground">Global controls for all scheduled jobs.</p>
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="sched-enabled" className="text-sm">
            {enabled ? 'Running' : 'Paused'}
          </Label>
          <Switch id="sched-enabled" checked={enabled} disabled={!canEdit || pausing} onCheckedChange={(v) => void setEnabled(v)} />
        </div>
      </div>

      {error && <p className="mb-2 text-xs text-destructive">{error}</p>}

      <fieldset disabled={!canEdit} className="grid items-start gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_auto]">
        <form.AppField name="alertWebhookUrl">
          {(f) => (
            <f.TextField
              id="alert-url"
              label="Failure alert webhook URL"
              // Stored as a secret, so it never comes back - a blank field keeps whatever is set
              placeholder={alertIsSet ? 'Configured - type a new URL to replace it' : 'https://hooks.example.com/scheduler-alerts'}
              className="text-sm"
              hint="POSTed when a job crosses its alert-after-failures threshold."
            />
          )}
        </form.AppField>
        <form.AppField name="historyRetentionDays">
          {(f) => <f.NumberField id="retention" label="Run-history retention (days)" min={1} />}
        </form.AppField>
        {canEdit && (
          <Button type="submit" variant="outline" size="sm" className="h-10 sm:mt-5.5" disabled={!dirty || submitting}>
            {submitting ? 'Saving…' : saved && !dirty ? 'Saved' : 'Save'}
          </Button>
        )}
      </fieldset>
    </form>
  )
}
