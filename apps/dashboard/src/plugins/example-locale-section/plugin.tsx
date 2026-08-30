// Example plugin - the minimum surface for extending a detail page without touching core: loader.ts auto-imports src/plugins/<name>/plugin.{ts,tsx}, and the import-time registerDetailSection call is the whole wiring

import { DetailPageSection } from '@/components/layout/detail-page'
import { registerDetailSection } from '@/plugins/detail-section-registry'

type LocaleEntity = { code: string; name: string } | null

registerDetailSection<LocaleEntity>({
  id: 'example:locale.notes',
  target: 'locale',
  // Over 50 keeps plugin sections below the core ones, which sit in the 10-50 band
  order: 100,
  mode: 'edit',
  render: ({ entity }) => (
    <DetailPageSection
      title="Example plugin"
      description="Rendered by src/plugins/example-locale-section. Replace with your own logic."
    >
      <p className="text-sm text-muted-foreground">
        Editing locale{' '}
        <code className="font-mono text-foreground">{entity?.code ?? '(none)'}</code>
        {entity?.name ? ` - ${entity.name}` : null}. A real plugin would render its
        own state, mutations, and Save button here.
      </p>
    </DetailPageSection>
  ),
})
