import { useEffect, useState } from 'react'

// Holds a value back until it stops changing - a server-paged list would otherwise fire a query per keystroke
export function useDebounced<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return settled
}
