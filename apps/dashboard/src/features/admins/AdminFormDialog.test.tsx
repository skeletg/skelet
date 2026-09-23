import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { AdminFormDialog } from './AdminFormDialog'
import { AdminsDocument, CreateAdminDocument } from './Admins.gql'
import { RolesDocument } from './roles/Roles.gql'

describe('AdminFormDialog', () => {
  it('checks the email and password before creating', async () => {
    const onCreate = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog(<AdminFormDialog open onOpenChange={onOpenChange} />, [
      { request: { query: RolesDocument }, result: { data: { roles: [] } }, maxUsageCount: 3 },
      {
        request: { query: CreateAdminDocument, variables: captureInput(onCreate) },
        result: { data: { createAdmin: { __typename: 'AdminUser', id: 'a1' } } },
      },
      { request: { query: AdminsDocument }, result: { data: { admins: [] } }, maxUsageCount: 3 },
    ])

    fireEvent.change(screen.getByLabelText('First name'), { target: { value: 'Ada' } })
    fireEvent.change(screen.getByLabelText('Last name'), { target: { value: 'Lovelace' } })
    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'ada@' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'short' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create admin' }))
    expect(await screen.findByText('Enter a valid email address')).toBeDefined()
    expect(screen.getByText('Minimum 8 characters')).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'ada@example.com' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create admin' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onCreate).toHaveBeenCalledWith({
      firstName: 'Ada',
      lastName: 'Lovelace',
      roleSlugs: [],
      email: 'ada@example.com',
      password: 'correct horse',
    })
  })
})
