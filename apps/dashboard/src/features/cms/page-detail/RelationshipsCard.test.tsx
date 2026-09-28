import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { LinkablesDocument, type CmsPageRowFragment } from '../Cms.gql'
import { RelationshipsCard } from './RelationshipsCard'
import type { PageTypeAttr } from './shared'

// What this pins: unlinking one entity writes only that attribute, with the rest in their curated order

const attr = (id: string) =>
  ({
    isRequired: false,
    attribute: { id, slug: id, inputType: 'RELATIONSHIP', relationshipTarget: 'PRODUCT', isRequiredDefault: false, content: { name: id }, values: [] },
  }) as unknown as PageTypeAttr

const linkable = (entityId: string, label: string) => ({ __typename: 'Linkable', entityType: 'PRODUCT', entityId, label, path: `/p/${entityId}` })

describe('RelationshipsCard', () => {
  it('writes only the changed attribute, keeping its order', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const relationships = [
      { attributeId: 'featured', targetType: 'PRODUCT', targetId: 'p1' },
      { attributeId: 'featured', targetType: 'PRODUCT', targetId: 'p2' },
      { attributeId: 'featured', targetType: 'PRODUCT', targetId: 'p3' },
      { attributeId: 'related', targetType: 'PRODUCT', targetId: 'p1' },
    ] as unknown as CmsPageRowFragment['relationships']
    renderSection(
      <RelationshipsCard pageId="pg1" relationshipAttrs={[attr('featured'), attr('related')]} relationships={relationships} canManage onSave={onSave} />,
      [
        {
          request: { query: LinkablesDocument, variables: { search: '', limit: 20 } },
          result: { data: { linkables: [linkable('p1', 'Mug'), linkable('p2', 'Cap'), linkable('p3', 'Tee')] } },
          maxUsageCount: 10,
        },
      ],
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Remove Cap' }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onSave).toHaveBeenCalled(), SLOW)
    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith('featured', [
      { targetType: 'PRODUCT', targetId: 'p1' },
      { targetType: 'PRODUCT', targetId: 'p3' },
    ])
  })
})
