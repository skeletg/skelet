import { useQuery } from '@apollo/client/react'
import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import { ADDRESS_FIELDS, toAddressValues } from '@/shared/schemas/address'
import { withFieldGroup } from './form'

// One postal address as a TanStack field group (shared/schemas/address) - drop it into any form that
// holds an address: <AddressFields form={form} fields="billing" idPrefix="bill" />
export const AddressFields = withFieldGroup({
  defaultValues: toAddressValues(),
  props: { idPrefix: 'address' },
  render: function Render({ group, idPrefix }) {
    // the country is a known list - typing it only ever produced a rejected save
    const countries = useQuery(CountriesPickerDocument).data?.countries ?? []
    return (
      <div className="grid grid-cols-2 items-start gap-3">
        {ADDRESS_FIELDS.map((f) => (
          <group.AppField key={f.key} name={f.key}>
            {(field) =>
              f.key === 'countryCode' ? (
                <field.SelectField
                  id={`${idPrefix}-${f.key}`}
                  label={`${f.label}${f.required ? ' *' : ''}`}
                  placeholder="Country"
                  noneLabel="- None -"
                  options={countries.map((c) => ({ value: c.code, label: `${c.emojiFlag ? `${c.emojiFlag} ` : ''}${c.name} (${c.code})` }))}
                  className={f.half ? '' : 'col-span-2'}
                />
              ) : (
                <field.TextField
                  id={`${idPrefix}-${f.key}`}
                  label={`${f.label}${f.required ? ' *' : ''}`}
                  className={f.half ? '' : 'col-span-2'}
                />
              )
            }
          </group.AppField>
        ))}
      </div>
    )
  },
})
