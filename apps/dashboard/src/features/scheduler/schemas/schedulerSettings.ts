import { z } from 'zod'
import { optionalAbsoluteUrl, wholeNumber } from '@/shared/schemas/fields'
import type { SchedulerConfigQuery } from '../Scheduler.gql'

// The scheduler's global config strip - THE field list (docs/conventions.md, "Record forms"). The
// pause switch isn't here: pausing is a lifecycle action and writes the moment it's flipped

export const PATH_ENABLED = 'system/scheduler/enabled'

const PATHS = {
  alertWebhookUrl: 'system/scheduler/alert_webhook_url',
  historyRetentionDays: 'system/scheduler/history_retention_days',
} as const

export const schedulerSettingsSchema = z
  .object({
    alertWebhookUrl: optionalAbsoluteUrl().describe('Absolute URL alerted when a scheduled job fails - empty string for none'),
    historyRetentionDays: wholeNumber('Whole days').min(1, 'At least one day').describe('Days to keep job run history, at least 1'),
  })
  .describe("The job scheduler's global settings")

export type SchedulerSettingsValues = z.infer<typeof schedulerSettingsSchema>

export function toSchedulerSettingsValues(config: SchedulerConfigQuery | undefined): SchedulerSettingsValues {
  return {
    alertWebhookUrl: config?.alertWebhookUrl?.value ?? '',
    historyRetentionDays: Number(config?.historyRetentionDays?.value) || 30,
  }
}

// One setConfigValue per changed field - config values are stored as strings
export function changedSchedulerConfig(before: SchedulerSettingsValues, after: SchedulerSettingsValues) {
  const out: { path: string; value: string }[] = []
  if (after.alertWebhookUrl.trim() !== before.alertWebhookUrl.trim()) {
    out.push({ path: PATHS.alertWebhookUrl, value: after.alertWebhookUrl.trim() })
  }
  if (after.historyRetentionDays !== before.historyRetentionDays) {
    out.push({ path: PATHS.historyRetentionDays, value: String(after.historyRetentionDays) })
  }
  return out
}
