import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { useAppForm } from '@/components/form/form'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Checkbox } from '@/components/ui/checkbox'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Label } from '@/components/ui/label'
import { LoadError } from '@/shared/errors/LoadError'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFormSection } from '@/shared/saving/useFormSection'
import {
  LocalesPickerDocument,
  SetChannelLocalesDocument,
} from '@/features/locales/Locales.gql'
import {
  ChannelDocument,
  ChannelsDocument,
  UpdateChannelDocument,
  type ChannelQuery,
} from '@/features/channels/Channels.gql'
import {
  channelLocalesDiff,
  channelLocalesSchema,
  channelLocalesSteps,
  toChannelLocalesValues,
} from './schemas/channelLocales'

// LocalesEditor combines the supported-locales picker (junction `channel_locales`) and the
// default-locale select - Edits are staged and applied through the SettingsPage header Save +
// SaveConfirmDialog (the dashboard-wide convention, see docs/conventions.md): confirming fires
// SetChannelLocalesDocument for the supported set and UpdateChannelDocument for the default,
// sequenced so the backend's "default must stay supported" guard is never violated mid-save
export function LocalesEditor({ channelId }: { channelId: string }) {
  const channelQuery = useQuery(ChannelDocument, {
    variables: { id: channelId },
    notifyOnNetworkStatusChange: true,
  })
  const channel = channelQuery.data?.channel

  if (channelQuery.loading && !channel) {
    return (
      <DetailPageSection description="Loading channel…">
        <div />
      </DetailPageSection>
    )
  }

  if (channelQuery.error || !channel) {
    return (
      <DetailPageSection>
        <LoadError
          title="Couldn't load channel"
          message={channelQuery.error?.message ?? 'Channel not found.'}
          onRetry={() => void channelQuery.refetch()}
          retrying={channelQuery.loading}
        />
      </DetailPageSection>
    )
  }

  // Keyed on the channel - switching channel in the header scope picker starts a fresh form
  return <LocalesForm key={channel.id} channel={channel} />
}

function LocalesForm({ channel }: { channel: NonNullable<ChannelQuery['channel']> }) {
  const permissions = usePermissions()
  const canEdit = permissions.canAccessChannelScope('channels.manage', channel.id)

  const localesQuery = useQuery(LocalesPickerDocument)
  const allLocales = useMemo(
    () => [...(localesQuery.data?.locales ?? [])].sort((a, b) => a.code.localeCompare(b.code)),
    [localesQuery.data],
  )

  const refetchQueries = [{ query: ChannelDocument, variables: { id: channel.id } }, { query: ChannelsDocument }]
  const [setChannelLocales] = useMutation(SetChannelLocalesDocument, { refetchQueries })
  const [updateChannel] = useMutation(UpdateChannelDocument, { refetchQueries })
  // A refused toggle - UI feedback, not the admin's work
  const [hint, setHint] = useState<string | null>(null)

  const baseline = useMemo(() => toChannelLocalesValues(channel), [channel])
  const form = useAppForm({
    defaultValues: baseline,
    validators: { onChange: channelLocalesSchema },
    onSubmit: async ({ value }) => {
      for (const step of channelLocalesSteps(baseline, value)) {
        if (step.kind === 'codes') {
          await setChannelLocales({ variables: { channelId: channel.id, codes: step.codes } })
        } else {
          await updateChannel({
            variables: {
              input: {
                id: channel.id,
                defaultLocale: step.code,
                name: channel.name,
                isActive: channel.isActive,
                currencyCode: channel.currencyCode,
                publicUrl: channel.publicUrl,
                metadata: channel.metadata,
              },
            },
          })
        }
      }
      form.reset(value)
    },
  })
  useFormSection({ id: 'channel-locales', title: 'Locales', form, baseline, diff: channelLocalesDiff })

  const codes = useStore(form.store, (s) => s.values.codes)
  const defaultLocale = useStore(form.store, (s) => s.values.defaultLocale)

  function toggleLocale(code: string) {
    setHint(null)
    if (!codes.includes(code)) {
      form.setFieldValue('codes', [...codes, code])
      return
    }
    // Never let the operator yank the (staged) default locale out of the supported set - the
    // backend would reject the save anyway. Surface a hint instead
    if (code === defaultLocale) {
      setHint(`${code} is the channel's default locale. Change the default first, then remove this entry.`)
      return
    }
    form.setFieldValue(
      'codes',
      codes.filter((c) => c !== code),
    )
  }

  const describe = (code: string) => {
    const entry = allLocales.find((l) => l.code === code)
    return (
      <>
        <span className="mr-1.5" aria-hidden>
          {entry?.flagEmoji || '🏳️'}
        </span>
        {entry?.nativeName || entry?.name || code} <span className="text-xs text-muted-foreground">({code})</span>
      </>
    )
  }

  return (
    <DetailPageSection description="Languages this channel serves. Apply edits via the header Save button.">
      <fieldset disabled={!canEdit} className="grid gap-4">
        <div className="grid gap-2">
          <Label>Supported locales</Label>
          {localesQuery.loading ? (
            <p className="text-xs text-muted-foreground">Loading languages…</p>
          ) : allLocales.length === 0 ? (
            <Alert>
              <AlertTitle>No active languages defined</AlertTitle>
              <AlertDescription>Create at least one language on the Languages page first, then assign it here.</AlertDescription>
            </Alert>
          ) : (
            <div className="grid gap-2 rounded-md border p-3 sm:grid-cols-2">
              {allLocales.map((entry) => (
                <label
                  key={entry.code}
                  className="flex cursor-pointer items-start gap-2 rounded px-2 py-1 hover:bg-muted/50 has-data-[state=checked]:bg-muted"
                >
                  <Checkbox
                    checked={codes.includes(entry.code)}
                    onCheckedChange={() => toggleLocale(entry.code)}
                    className="mt-0.5"
                    aria-label={`Support ${entry.code}`}
                  />
                  <div className="grid gap-0.5 leading-tight">
                    <div className="flex items-center gap-2 text-sm">
                      <span className="text-base leading-none" aria-hidden>
                        {entry.flagEmoji || '🏳️'}
                      </span>
                      <span className="font-medium">{entry.nativeName || entry.name}</span>
                      <code className="font-mono text-xs text-muted-foreground">({entry.code})</code>
                    </div>
                    {entry.nativeName && entry.name !== entry.nativeName && (
                      <span className="pl-7 text-xs text-muted-foreground">{entry.name}</span>
                    )}
                  </div>
                </label>
              ))}
            </div>
          )}
        </div>

        <form.AppField name="defaultLocale">
          {(f) => (
            <f.SelectField
              id="locales-default"
              label="Default locale"
              className="max-w-sm"
              disabled={codes.length === 0 || !canEdit}
              placeholder={codes.length === 0 ? 'Select supported locales first' : 'Pick a default…'}
              options={codes.map((code) => ({ value: code, label: describe(code) }))}
              hint="Only locales selected above are eligible."
            />
          )}
        </form.AppField>

        {!canEdit && (
          <Alert>
            <AlertTitle>Read-only</AlertTitle>
            <AlertDescription>You don't have channels.manage on this channel.</AlertDescription>
          </Alert>
        )}

        {hint && (
          <Alert variant="destructive">
            <AlertTitle>Can't stage that</AlertTitle>
            <AlertDescription>{hint}</AlertDescription>
          </Alert>
        )}
      </fieldset>
    </DetailPageSection>
  )
}
