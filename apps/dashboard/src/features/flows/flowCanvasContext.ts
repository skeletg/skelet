import { createContext } from 'react'
import type { FlowNames } from './flowTypes'

export type FlowCanvas = { names: FlowNames; flags: Map<string, 'error' | 'warning'> }

export const FlowCanvasContext = createContext<FlowCanvas>({ names: { emails: new Map(), segments: new Map() }, flags: new Map() })
