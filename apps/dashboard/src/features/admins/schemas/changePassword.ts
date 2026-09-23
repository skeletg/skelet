import { z } from 'zod'

// Self-service password change. The length floor mirrors the backend's auth.MinPasswordLen so it
// can be said up front rather than bounced back from the server

export const MIN_PASSWORD_LENGTH = 8

export const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, 'Enter your current password'),
    newPassword: z.string().min(MIN_PASSWORD_LENGTH, `At least ${MIN_PASSWORD_LENGTH} characters`),
    confirmPassword: z.string(),
  })
  .refine((v) => v.confirmPassword === v.newPassword, { message: "Passwords don't match", path: ['confirmPassword'] })
  .refine((v) => !v.newPassword || v.newPassword !== v.currentPassword, {
    message: 'Has to be different from the current password',
    path: ['newPassword'],
  })

export type ChangePasswordValues = z.infer<typeof changePasswordSchema>

export const emptyChangePassword: ChangePasswordValues = { currentPassword: '', newPassword: '', confirmPassword: '' }
