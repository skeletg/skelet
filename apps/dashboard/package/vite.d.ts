import type { UserConfig } from 'vite'

// A project's whole dashboard vite.config. plugins are the npm packages of its plugins' dashboard halves; connect and
// frame are origins those plugins reach or frame, added to the build's CSP beside the ones .env already names
export declare function skeletDashboard(options?: { plugins?: string[]; connect?: string[]; frame?: string[] }): UserConfig
