import type { ReactNode } from 'react'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { useSaveConfirm } from '@/shared/saving/useSaveConfirm'

// A section hosted the way its page would host it: the registry, a header "Save changes" and the
// confirm dialog it opens - see renderSection in ./page-harness
export function SectionHost({ children }: { children: ReactNode }) {
  const registry = useSaveRegistry('Section')
  const confirm = useSaveConfirm(registry.saveAll)
  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <button type="button" disabled={!registry.isDirty} onClick={confirm.open}>
        Save changes
      </button>
      {children}
      <SaveConfirmDialog {...confirm.dialogProps} scopeLabel="Section" sections={registry.dirtySections} />
    </SaveRegistryContext.Provider>
  )
}
