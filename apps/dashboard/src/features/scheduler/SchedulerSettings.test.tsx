import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderDialog } from '@/test/page-harness'
import { SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { SchedulerConfigDocument } from './Scheduler.gql'
import { DEFAULT_SCOPE } from './scheduler-config'
import { SchedulerSettings } from './SchedulerSettings'

// What these pin: a relative webhook URL is refused before anything is written, and one Save writes
// only the config values that changed

const config = {
  enabled: { __typename: 'ConfigValue', value: 'true', isDefault: true },
  alertWebhookUrl: { __typename: 'ConfigValue', value: '', isDefault: true },
  historyRetentionDays: { __typename: 'ConfigValue', value: '30', isDefault: true },
}

describe('SchedulerSettings', () => {
  it('refuses a relative URL, then writes only the changed value', async () => {
    const writes = vi.fn()
    renderDialog(<SchedulerSettings />, [
      meMock(),
      { request: { query: SchedulerConfigDocument, variables: { scope: DEFAULT_SCOPE } }, result: { data: config }, maxUsageCount: 5 },
      {
        request: {
          query: SetConfigValueDocument,
          variables: (vars: Record<string, unknown>) => {
            writes(vars)
            return true
          },
        },
        result: {
          data: {
            setConfigValue: { __typename: 'ConfigValue', path: 'x', value: 'x', isDefault: false, scope: { __typename: 'ConfigScope', kind: 'DEFAULT', channelId: null, locale: null } },
          },
        },
      },
    ])

    fireEvent.change(await screen.findByLabelText('Failure alert webhook URL'), { target: { value: 'hooks.example.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/Enter a full URL/)).toBeDefined()
    expect(writes).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Failure alert webhook URL'), { target: { value: 'https://hooks.example.com/a' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(() => expect(writes).toHaveBeenCalledTimes(1), SLOW)
    expect(writes).toHaveBeenCalledWith(
      expect.objectContaining({ path: 'system/scheduler/alert_webhook_url', value: 'https://hooks.example.com/a' }),
    )
  })
})
