// Starts the dashboard in el. Call configure() from @skelet/dashboard/config first, then import this dynamically, so
// it doesn't load before the configuration - see the template's src/main.ts. plugins loads the project's plugins once
// the dashboard's own menu and registries exist, and before the first render
export declare function mount(el: HTMLElement, options?: { plugins?: () => Promise<unknown> }): Promise<void>
