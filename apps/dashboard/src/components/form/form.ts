import { createFormHook } from '@tanstack/react-form'
import { fieldContext, formContext } from './form-context'
import { CheckboxField, MetadataField, NumberField, SwitchField, TextareaField, TextField } from './fields'

// The dashboard's form hook - TanStack Form with our bound field components registered, so pages
// write <form.AppField name="city">{(f) => <f.TextField label="City" />}</form.AppField>
//
// Forms on a detail page still save through the page-header Save: wire the form into the registry
// with shared/saving/useFormSection, and into refresh-proof drafts with usePersistedForm
export const { useAppForm, withForm } = createFormHook({
  fieldContext,
  formContext,
  fieldComponents: { TextField, TextareaField, NumberField, SwitchField, CheckboxField, MetadataField },
  formComponents: {},
})
