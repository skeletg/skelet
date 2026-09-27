import { defineConfig } from '@lingui/cli'

// English is the source locale and ships inlined by the macro, so no locale ever compiles into
// the bundle here - `pnpm run i18n:extract` writes src/locales/messages.pot, the template language
// packs translate (see apps/backend/sdk/translations.go for how packs reach the dashboard)
export default defineConfig({
  sourceLocale: 'en',
  locales: ['en'],
  catalogs: [
    {
      path: '<rootDir>/src/locales/{locale}',
      include: ['src'],
    },
  ],
})
