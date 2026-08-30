const handlers = new Map<string, () => void>() // nav leaves register at module load, before any provider mounts - so an action's handler is looked up at click time rather than stored on the leaf

export function setNavAction(id: string, run: () => void): () => void {
  handlers.set(id, run)
  return () => {
    if (handlers.get(id) === run) handlers.delete(id)
  }
}

export function runNavAction(id: string) {
  handlers.get(id)?.()
}
