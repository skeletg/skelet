import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { usePermissions } from '@/shared/auth/usePermissions'
import { SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { SchedulerConfigDocument } from './Scheduler.gql'
import { DEFAULT_SCOPE } from './scheduler-config'

const PATH_ENABLED = 'system/scheduler/enabled'
const PATH_ALERT = 'system/scheduler/alert_webhook_url'
const PATH_RETENTION = 'system/scheduler/history_retention_days'

/**
 * SchedulerSettings is the global config strip: master pause switch, failure
 * alert webhook, and run-history retention - Writes go straight through
 * setConfigValue (immediate, like the other settings/list pages) and refetch
 * the shared SchedulerConfig query so the paused banner stays in sync
 */
export function SchedulerSettings() {
  const { has } = usePermissions()
  const canView = has('config.view')
  const canEdit = has('config.manage_defaults')
  const { data } = useQuery(SchedulerConfigDocument, {
    variables: { scope: DEFAULT_SCOPE },
    skip: !canView,
  })
  const [setConfigValue, { loading: saving }] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: SchedulerConfigDocument, variables: { scope: DEFAULT_SCOPE } }],
  })

  const enabled = data?.enabled?.value !== 'false'
  const [alertUrl, setAlertUrl] = useState('')
  const [retention, setRetention] = useState('30')
  const [savedField, setSavedField] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

  // Seed local inputs once the config loads (state adjustment during
  // render, keyed on the config objects themselves)
  const alertCfg = data?.alertWebhookUrl
  const retentionCfg = data?.historyRetentionDays
  const [prevCfg, setPrevCfg] = useState<{
    alert: typeof alertCfg
    retention: typeof retentionCfg
  }>({ alert: undefined, retention: undefined })
  if (prevCfg.alert !== alertCfg || prevCfg.retention !== retentionCfg) {
    setPrevCfg({ alert: alertCfg, retention: retentionCfg })
    if (alertCfg) setAlertUrl(alertCfg.value)
    if (retentionCfg) setRetention(retentionCfg.value)
  }

  async function save(path: string, value: string, fieldLabel: string) {
    setErr(null)
    try {
      await setConfigValue({ variables: { path, value, scope: DEFAULT_SCOPE } })
      setSavedField(fieldLabel)
      setTimeout(() => setSavedField((s) => (s === fieldLabel ? null : s)), 1500)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Save failed.')
    }
  }

  // Global settings are config-backed; hide the strip entirely from admins
  // without config visibility (they keep per-job scheduler controls)
  if (!canView) return null

  return (
    <div className="rounded-md border p-4">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">Scheduler settings</h2>
          <p className="text-xs text-muted-foreground">Global controls for all scheduled jobs.</p>
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="sched-enabled" className="text-sm">
            {enabled ? 'Running' : 'Paused'}
          </Label>
          <Switch
            id="sched-enabled"
            checked={enabled}
            disabled={!canEdit || saving}
            onCheckedChange={(v) => save(PATH_ENABLED, v ? 'true' : 'false', 'enabled')}
          />
        </div>
      </div>

      {err && <p className="mb-2 text-xs text-destructive">{err}</p>}

      <div className="grid gap-3 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="grid gap-1.5">
          <Label htmlFor="alert-url" className="text-xs">Failure alert webhook URL</Label>
          <div className="flex gap-2">
            <Input
              id="alert-url"
              value={alertUrl}
              onChange={(e) => setAlertUrl(e.target.value)}
              placeholder="https://hooks.example.com/scheduler-alerts"
              disabled={!canEdit}
              className="text-sm"
            />
            <Button
              variant="outline"
              size="sm"
              disabled={!canEdit || saving || alertUrl === (data?.alertWebhookUrl?.value ?? '')}
              onClick={() => save(PATH_ALERT, alertUrl.trim(), 'alert')}
            >
              {savedField === 'alert' ? 'Saved' : 'Save'}
            </Button>
          </div>
          <p className="text-[11px] leading-tight text-muted-foreground">
            POSTed (HMAC-signed) when a job crosses its alert-after-failures threshold.
          </p>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="retention" className="text-xs">Run-history retention (days)</Label>
          <div className="flex gap-2">
            <Input
              id="retention"
              type="number"
              min={1}
              value={retention}
              onChange={(e) => setRetention(e.target.value)}
              disabled={!canEdit}
            />
            <Button
              variant="outline"
              size="sm"
              disabled={!canEdit || saving || retention === (data?.historyRetentionDays?.value ?? '')}
              onClick={() => save(PATH_RETENTION, String(Math.max(1, Number(retention) || 30)), 'retention')}
            >
              {savedField === 'retention' ? 'Saved' : 'Save'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
