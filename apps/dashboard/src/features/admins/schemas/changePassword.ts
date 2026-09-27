import { z } from 'zod'
import { newPassword } from '@/shared/schemas/fields'

// Self-service password change

export const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, 'Enter your current password').describe("The signed-in admin's current password"),
    newPassword: newPassword(),
    confirmPassword: z.string().describe('The new password typed again - must match it exactly'),
  })
  .describe("A change to the signed-in admin's own password")
  .refine((v) => v.confirmPassword === v.newPassword, { message: "Passwords don't match", path: ['confirmPassword'] })
  .refine((v) => !v.newPassword || v.newPassword !== v.currentPassword, {
    message: 'Has to be different from the current password',
    path: ['newPassword'],
  })

export type ChangePasswordValues = z.infer<typeof changePasswordSchema>

export const emptyChangePassword: ChangePasswordValues = { currentPassword: '', newPassword: '', confirmPassword: '' }
