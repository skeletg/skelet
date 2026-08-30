import { useCallback, useState } from 'react'

// Split from SchemaForm so that file exports only components - fast refresh needs it

// Convenience for managing a JSON blob in a controlled SchemaForm - unused by
// PromotionRuleForm, which manages state inline, but handy for plugin authors
export function useFormState<T>(initial: T) {
  const [value, setValue] = useState<T>(initial)
  const reset = useCallback(() => setValue(initial), [initial])
  return [value, setValue, reset] as const
}
