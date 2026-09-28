import { useState } from 'react'

// The header-Save -> SaveConfirmDialog round trip every staged-save page repeats: open the dialog,
// run saveAll on confirm, keep it open with the reason if anything throws
//
//   const confirm = useSaveConfirm(saveAll)
//   <Button onClick={confirm.open} disabled={!isDirty}>Save</Button>
//   <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={…} sections={dirtySections} />
export function useSaveConfirm(saveAll: () => Promise<unknown>) {
  const [isOpen, setIsOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function onConfirm() {
    setError(null)
    setSaving(true)
    try {
      await saveAll()
      setIsOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  function onCancel() {
    if (saving) return
    setIsOpen(false)
    setError(null)
  }

  return {
    open: () => setIsOpen(true),
    saving,
    dialogProps: { open: isOpen, saving, error, onConfirm, onCancel },
  }
}
