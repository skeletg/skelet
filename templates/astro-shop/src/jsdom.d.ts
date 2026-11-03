// jsdom ships no types and @types/jsdom isn't installed - the page tests only build a window from HTML
declare module "jsdom" {
  export class JSDOM {
    constructor(html?: string);
    readonly window: Window & typeof globalThis;
  }
}
