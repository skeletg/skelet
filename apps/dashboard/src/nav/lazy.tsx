import { lazy, type ComponentType } from 'react'

// Route-level code splitting - each page becomes its own chunk, so a heavy library only one page uses (recharts, xyflow, lexical) never lands in the eager bundle
export function lazyPage<K extends string, P extends object>(
  load: () => Promise<Record<K, ComponentType<P>>>,
  name: K,
  props?: NoInfer<P>,
) {
  const Page = lazy(async () => ({ default: (await load())[name] })) as unknown as ComponentType<P> // the cast sidesteps LazyExoticComponent attribute inference, which chokes on a generic P
  return <Page {...((props ?? {}) as P)} />
}
