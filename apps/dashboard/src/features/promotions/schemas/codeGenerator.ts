import { z } from 'zod'
import { wholeNumber } from '@/shared/schemas/fields'

// Bulk promotion-code generator. Pattern grammar: [A] uppercase letter, [N] digit, [*] alphanumeric;
// anything outside the brackets is literal, and prefix + suffix wrap the whole pattern -
// "WELCOME-[N][N][N]" yields WELCOME-481, WELCOME-237, …

export const codeGeneratorSchema = z
  .object({
    // Without a placeholder every generated code would be the same literal string
    pattern: z
      .string()
      .refine((v) => /\[(A|N|\*)\]/.test(v), 'Needs at least one [A], [N] or [*] placeholder')
      .describe(
        'Code pattern: [A] an uppercase letter, [N] a digit, [*] a letter or digit, anything else literal - such as WELCOME-[N][N][N]. Needs at least one placeholder',
      ),
    count: wholeNumber().min(1, 'At least 1').max(10000, 'At most 10000').describe('How many codes to generate, 1 to 10000'),
    prefix: z.string().describe('Text before every code - empty string for none'),
    suffix: z.string().describe('Text after every code - empty string for none'),
  })
  .describe('A batch of promotion codes to generate for a rule')

export type CodeGeneratorValues = z.infer<typeof codeGeneratorSchema>

export const defaultCodeGenerator: CodeGeneratorValues = { pattern: '[A][A][N][N][N][N]', count: 10, prefix: '', suffix: '' }

export function toCodeGeneratorVariables(v: CodeGeneratorValues, ruleId: string) {
  return { ruleId, pattern: v.pattern, count: v.count, prefix: v.prefix || null, suffix: v.suffix || null }
}
