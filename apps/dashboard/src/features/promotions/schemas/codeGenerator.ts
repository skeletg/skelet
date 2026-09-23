import { z } from 'zod'

// Bulk promotion-code generator. Pattern grammar: [A] uppercase letter, [N] digit, [*] alphanumeric;
// anything outside the brackets is literal, and prefix + suffix wrap the whole pattern -
// "WELCOME-[N][N][N]" yields WELCOME-481, WELCOME-237, …

export const codeGeneratorSchema = z.object({
  // Without a placeholder every generated code would be the same literal string
  pattern: z.string().refine((v) => /\[(A|N|\*)\]/.test(v), 'Needs at least one [A], [N] or [*] placeholder'),
  count: z.number().int('Whole numbers only').min(1, 'At least 1').max(10000, 'At most 10000'),
  prefix: z.string(),
  suffix: z.string(),
})

export type CodeGeneratorValues = z.infer<typeof codeGeneratorSchema>

export const defaultCodeGenerator: CodeGeneratorValues = { pattern: '[A][A][N][N][N][N]', count: 10, prefix: '', suffix: '' }

export function toCodeGeneratorVariables(v: CodeGeneratorValues, ruleId: string) {
  return { ruleId, pattern: v.pattern, count: v.count, prefix: v.prefix || null, suffix: v.suffix || null }
}
