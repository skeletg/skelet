import { fireEvent, screen } from '@testing-library/react'
import { GraphQLError } from 'graphql'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { ChangePasswordDialog } from './ChangePasswordDialog'
import { ChangePasswordDocument } from './Admins.gql'

function fill(current: string, next: string, confirm: string) {
  fireEvent.change(screen.getByLabelText('Current password'), { target: { value: current } })
  fireEvent.change(screen.getByLabelText('New password'), { target: { value: next } })
  fireEvent.change(screen.getByLabelText('Confirm new password'), { target: { value: confirm } })
  fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
}

describe('ChangePasswordDialog', () => {
  it('checks the pair locally, then confirms in place', async () => {
    const onChange = vi.fn()
    renderDialog(<ChangePasswordDialog open onOpenChange={() => {}} />, [
      {
        request: {
          query: ChangePasswordDocument,
          variables: (vars: { currentPassword: string; newPassword: string }) => {
            onChange(vars)
            return true
          },
        },
        result: { data: { changePassword: true } },
      },
    ])

    fill('old-secret', 'new-secret-1', 'new-secret-2')
    expect(await screen.findByText("Passwords don't match")).toBeDefined()
    fill('same-secret', 'same-secret', 'same-secret')
    expect(await screen.findByText('Has to be different from the current password')).toBeDefined()
    expect(onChange).not.toHaveBeenCalled()

    fill('old-secret', 'new-secret-1', 'new-secret-1')
    expect(await screen.findByText('Password changed', {}, SLOW)).toBeDefined()
    expect(onChange).toHaveBeenCalledWith({ currentPassword: 'old-secret', newPassword: 'new-secret-1' })
  })

  it('keeps a wrong current password on screen', async () => {
    renderDialog(<ChangePasswordDialog open onOpenChange={() => {}} />, [
      { request: { query: ChangePasswordDocument, variables: () => true }, result: { errors: [new GraphQLError('current password is incorrect')] } },
    ])
    fill('wrong', 'new-secret-1', 'new-secret-1')
    expect(await screen.findByText('current password is incorrect', {}, SLOW)).toBeDefined()
  })
})
