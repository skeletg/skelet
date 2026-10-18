import { useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { ADDRESS_FIELDS, toAddressValues } from '@/shared/schemas/address'
import { withFieldGroup } from './form'

// One postal address as a TanStack field group (shared/schemas/address) - drop it into any form that
// holds an address: <AddressFields form={form} fields="billing" idPrefix="bill" />
export const AddressFields = withFieldGroup({
  defaultValues: toAddressValues(),
  // Types only - withFieldGroup reads the shape, the defaults live in the render. idPrefix is required:
  // two groups on one page (billing + shipping) would otherwise both render address-firstName
  props: {} as { idPrefix: string; optionalNames?: boolean },
  render: function Render({ group, idPrefix, optionalNames = false }) {
    // the country is a known list - typing it only ever produced a rejected save
    const countries = useQuery(CountriesPickerDocument).data?.countries ?? []
    // The phone is formatted and checked in the address's own country
    const country = useStore(group.store, (s) => s.values.countryCode)
    return (
      <div className="grid grid-cols-2 items-start gap-3">
        {ADDRESS_FIELDS.map((f) => {
          // A draft order can leave the names for later - they aren't marked required there
          const required = f.required && !(optionalNames && (f.key === 'firstName' || f.key === 'lastName'))
          const label = required ? `${f.label} *` : f.label
          const className = f.half ? '' : 'col-span-2'
          return (
            <group.AppField key={f.key} name={f.key}>
              {(field) =>
                f.key === 'countryCode' ? (
                  <field.SelectField
                    id={`${idPrefix}-${f.key}`}
                    label={label}
                    placeholder="Country"
                    noneLabel="- None -"
                    options={countries.map((c) => ({ value: c.code, label: `${c.emojiFlag ? `${c.emojiFlag} ` : ''}${c.name} (${c.code})` }))}
                    className={className}
                  />
                ) : f.key === 'phone' ? (
                  <field.PhoneField id={`${idPrefix}-${f.key}`} label={label} country={country} className={className} />
                ) : (
                  <field.TextField id={`${idPrefix}-${f.key}`} label={label} className={className} />
                )
              }
            </group.AppField>
          )
        })}
      </div>
    )
  },
})
