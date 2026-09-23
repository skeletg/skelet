import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { CodeGeneratorDialog } from './CodeGeneratorDialog'
import { GeneratePromotionCodesDocument } from './Promotions.gql'

describe('CodeGeneratorDialog', () => {
  it('refuses a pattern with no placeholder, then generates', async () => {
    const onVars = vi.fn()
    const onGenerated = vi.fn()
    renderDialog(<CodeGeneratorDialog ruleId="r1" open onOpenChange={() => {}} onGenerated={onGenerated} />, [
      {
        request: {
          query: GeneratePromotionCodesDocument,
          variables: (vars: Record<string, unknown>) => {
            onVars(vars)
            return true
          },
        },
        result: { data: { generatePromotionCodes: [] } },
      },
    ])
    fireEvent.change(screen.getByLabelText('Pattern'), { target: { value: 'WELCOME' } })
    fireEvent.click(screen.getByRole('button', { name: 'Generate' }))
    expect(await screen.findByText('Needs at least one [A], [N] or [*] placeholder')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Pattern'), { target: { value: 'WELCOME-[N][N][N]' } })
    fireEvent.change(screen.getByLabelText('Count'), { target: { value: '50' } })
    fireEvent.click(screen.getByRole('button', { name: 'Generate' }))
    await vi.waitFor(() => expect(onGenerated).toHaveBeenCalled(), SLOW)
    expect(onVars).toHaveBeenCalledWith({ ruleId: 'r1', pattern: 'WELCOME-[N][N][N]', count: 50, prefix: null, suffix: null })
  })
})
