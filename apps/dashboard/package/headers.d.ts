import type { Plugin } from 'vite'

export declare function originsFrom(env: Record<string, string | undefined>): { connect: string[]; frame: string[] }
export declare function renderHeaders(template: string, connect: string[], frame: string[]): string
export declare function securityHeaders(options: { template: string; connect?: string[]; frame?: string[] }): Plugin
