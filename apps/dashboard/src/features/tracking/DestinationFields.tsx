import { useStore } from '@tanstack/react-form'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { DetailPageSection } from '@/components/layout/detail-page'
import type { useRecordForm } from '@/shared/saving/useRecordForm'
import { ALL_EVENTS, eventLabel } from './platformGuides'
import type { PlatformField, TrackingDestinationValues } from './schemas/trackingDestination'

type Form = ReturnType<typeof useRecordForm<TrackingDestinationValues>>['form']

// The settings a platform asks for, in its own words, and which events it gets - shared by the guided setup and the
// destination page, so both read the same
export function DestinationFields({
  form,
  fields,
  defaultEvents,
  secretSet,
  idPrefix,
}: {
  form: Form
  fields: PlatformField[]
  defaultEvents: string[]
  secretSet: boolean
  idPrefix: string
}) {
  const useDefaults = useStore(form.store, (s) => s.values.useDefaultEvents)
  const events = useStore(form.store, (s) => s.values.events)
  const offered = Array.from(new Set([...defaultEvents, ...ALL_EVENTS]))

  return (
    <>
      <DetailPageSection title="Settings" description="Where to find each value is under the field. Credentials are stored encrypted and never shown again.">
        <div className="grid max-w-2xl gap-4">
          <form.AppField name="name">
            {(f) => <f.TextField id={`${idPrefix}-name`} label="Name" hint="Yours to recognise it by - several destinations of one platform can run side by side" />}
          </form.AppField>
          {fields.map((field) => (
            <form.AppField key={field.key} name={field.secret ? 'secret' : `settings.${field.key}`}>
              {(f) => (
                <f.TextField
                  id={`${idPrefix}-${field.key}`}
                  label={field.required ? field.label : `${field.label} (optional)`}
                  hint={field.help || undefined}
                  type={field.secret ? 'password' : 'text'}
                  autoComplete={field.secret ? 'new-password' : 'off'}
                  placeholder={field.secret && secretSet ? 'Set - leave empty to keep it' : undefined}
                />
              )}
            </form.AppField>
          ))}
        </div>
      </DetailPageSection>

      <DetailPageSection title="Events" description="Which shop events go to this destination. Consent still applies to each one.">
        <div className="grid max-w-2xl gap-3">
          <div className="flex items-center gap-3">
            <Switch
              id={`${idPrefix}-defaults`}
              checked={useDefaults}
              onCheckedChange={(on) => form.setFieldValue('useDefaultEvents', on)}
            />
            <Label htmlFor={`${idPrefix}-defaults`} className="font-normal">
              Send the platform's defaults
              {defaultEvents.length > 0 && <span className="text-muted-foreground"> - {defaultEvents.map(eventLabel).join(', ')}</span>}
            </Label>
          </div>
          {!useDefaults && (
            <div className="grid grid-cols-2 gap-2 rounded-md border p-3 sm:grid-cols-3">
              {offered.map((name) => {
                const id = `${idPrefix}-event-${name}`
                return (
                  <div key={name} className="flex items-center gap-2">
                    <Checkbox
                      id={id}
                      checked={events.includes(name)}
                      onCheckedChange={(on) =>
                        form.setFieldValue('events', on === true ? [...events, name] : events.filter((e) => e !== name))
                      }
                    />
                    <Label htmlFor={id} className="font-normal">{eventLabel(name)}</Label>
                  </div>
                )
              })}
            </div>
          )}
          <form.Subscribe selector={(s) => s.fieldMeta.events?.errors}>
            {(errors) => (errors?.length ? <p className="text-sm text-destructive">{String(errors[0]?.message ?? errors[0])}</p> : null)}
          </form.Subscribe>
        </div>
      </DetailPageSection>
    </>
  )
}
