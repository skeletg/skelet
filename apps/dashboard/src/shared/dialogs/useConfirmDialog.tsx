import { useCallback, useState, type ComponentProps } from 'react'
import { ConfirmDialog } from './ConfirmDialog'

type ConfirmRequest = Omit<ComponentProps<typeof ConfirmDialog>, 'open' | 'onOpenChange'>

// window.confirm's ergonomics with the dashboard's dialog: ask({ title, onConfirm, ... }) from any handler, render the
// returned element once. The last request stays mounted after closing so the close animation can play
export function useConfirmDialog() {
  const [state, setState] = useState<{ open: boolean; request: ConfirmRequest | null }>({ open: false, request: null })
  const ask = useCallback((request: ConfirmRequest) => setState({ open: true, request }), [])
  const dialog = state.request ? (
    <ConfirmDialog {...state.request} open={state.open} onOpenChange={(open) => setState((s) => ({ ...s, open }))} />
  ) : null
  return [ask, dialog] as const
}
