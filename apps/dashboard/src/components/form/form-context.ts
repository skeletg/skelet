import { createFormHookContexts } from '@tanstack/react-form'

// Own module so the field components and the hook factory can both import the contexts without
// importing each other - TanStack Form's composition pattern (createFormHook), see form.ts
export const { fieldContext, formContext, useFieldContext, useFormContext } = createFormHookContexts()
