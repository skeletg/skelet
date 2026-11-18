import type { ComponentType } from 'react'

// A plugin's own section on Home - one the admin arranges, hides and finds in Preferences like a core section, gated on
// a permission or feature switch of its own. The 'home:widgets' slot still renders below them for a plugin that only
// wants to add a line to the page

export type PluginHomeWidget = {
  id: string // '<plugin-name>:<section>' - the admin's saved arrangement keys on it, so renaming one drops it from it
  title: string // names the section in the arrange lists - the section titles itself on the page
  permission?: string
  feature?: string
  defaultHidden?: boolean // in the library until an admin switches it on, rather than on every fresh account's Home
  Component: ComponentType
}

const widgets: PluginHomeWidget[] = []

export function registerHomeWidget(widget: PluginHomeWidget) {
  if (!widget.id.includes(':')) {
    throw new Error(`home-widgets: "${widget.id}" needs a "<plugin>:" prefix, so it never takes a core section's place`)
  }
  if (widgets.some((w) => w.id === widget.id)) {
    throw new Error(`home-widgets: duplicate id "${widget.id}"`)
  }
  widgets.push(widget)
}

export function getHomeWidgets(): PluginHomeWidget[] {
  return [...widgets]
}

export function __resetHomeWidgetsForTests() {
  widgets.length = 0
}
