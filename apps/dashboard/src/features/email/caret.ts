// restore() puts the caret back after the tag once React has rendered the new value
export function insertAtCaret(id: string, current: string, tag: string): { value: string; restore: () => void } {
  const el = document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement | null
  const token = `{{${tag}}}`
  const start = el?.selectionStart ?? current.length
  const end = el?.selectionEnd ?? current.length
  const caret = start + token.length
  return {
    value: current.slice(0, start) + token + current.slice(end),
    restore: () =>
      requestAnimationFrame(() => {
        el?.focus()
        el?.setSelectionRange(caret, caret)
      }),
  }
}
