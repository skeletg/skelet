import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { usePermissions } from '@/shared/auth/usePermissions'
import { AssistantNameDocument } from './Agentic.gql'

export const DEFAULT_NAME = 'The shopping assistant'

// Word by word as a model answers - from the start whenever the key changes, whole at once where motion is reduced
export function useStreamed(text: string, key: string): { text: string; streaming: boolean } {
  const words = useMemo(() => text.split(/(?<=\s)/), [text])
  const still = calm()
  const [state, setState] = useState({ key: '', shown: 0 })
  const shown = still ? words.length : state.key === key ? state.shown : 0
  useEffect(() => {
    if (still || shown >= words.length) return
    const id = window.setTimeout(() => setState({ key, shown: shown + 1 }), 45)
    return () => window.clearTimeout(id)
  }, [still, key, shown, words.length])
  return { text: words.slice(0, shown).join(''), streaming: shown < words.length }
}

export function calm(): boolean {
  return typeof window.matchMedia !== 'function' || window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export function useAssistantName(channelId: string | undefined): string {
  const { canAccessChannelScope } = usePermissions()
  const { data } = useQuery(AssistantNameDocument, {
    variables: { scope: { kind: 'CHANNEL', channelId } },
    skip: !channelId || !canAccessChannelScope('config.view', channelId),
    fetchPolicy: 'cache-first',
  })
  return data?.configValue.value.trim() || DEFAULT_NAME
}

export function useTyping(ms = 750): [boolean, () => void] {
  const [typing, setTyping] = useState(false)
  const timer = useRef(0)
  const ping = useCallback(() => {
    setTyping(true)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setTyping(false), ms)
  }, [ms])
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return [typing, ping]
}
