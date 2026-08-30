const modules = import.meta.glob('./*/plugin.{ts,tsx}', { eager: true }) // eager keeps registration order deterministic and fills the registry before App.tsx renders

export const loadedPluginCount = Object.keys(modules).length // forces the glob into a runtime no-op so dead-code elimination can't drop it - the side effects ARE the registration
