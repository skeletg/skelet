import { Trans } from '@lingui/react/macro'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import {
  ChevronsUpDownIcon,
  CompassIcon,
  KeyRoundIcon,
  LogOutIcon,
  SlidersHorizontalIcon,
  UserRoundIcon,
} from 'lucide-react'
import { useAuth } from '@/shared/auth/auth'
import { ChangePasswordDialog } from '@/features/admins/ChangePasswordDialog'
import { useWelcome } from '@/features/onboarding/welcome-context'

function initials(firstName: string, lastName: string) {
  return `${firstName[0] ?? ''}${lastName[0] ?? ''}`.toUpperCase() || '?'
}

export function NavUser() {
  const { auth, setAuth } = useAuth()
  const { isMobile } = useSidebar()
  const navigate = useNavigate()
  const welcome = useWelcome() // null outside the shell, so the item simply isn't offered
  const [changingPassword, setChangingPassword] = useState(false)

  if (!auth) return null
  const { admin } = auth
  const displayName = `${admin.firstName} ${admin.lastName}`.trim() || admin.email

  function onLogout() {
    setAuth(null)
    navigate('/login', { replace: true })
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              data-tour="user"
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
            >
              <Avatar className="h-8 w-8 rounded-lg">
                <AvatarFallback className="rounded-lg">
                  {initials(admin.firstName, admin.lastName)}
                </AvatarFallback>
              </Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{displayName}</span>
                <span className="truncate text-xs">{admin.email}</span>
              </div>
              <ChevronsUpDownIcon className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
            side={isMobile ? 'bottom' : 'right'}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                <Avatar className="h-8 w-8 rounded-lg">
                  <AvatarFallback className="rounded-lg">
                    {initials(admin.firstName, admin.lastName)}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-medium">{displayName}</span>
                  <span className="truncate text-xs">{admin.email}</span>
                </div>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => navigate(`/admins/${admin.id}`)}>
              <UserRoundIcon />
              <Trans>Account</Trans>
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setChangingPassword(true)}>
              <KeyRoundIcon />
              <Trans>Change password</Trans>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => navigate('/preferences')}>
              <SlidersHorizontalIcon />
              <Trans>Preferences</Trans>
            </DropdownMenuItem>
            {welcome && (
              <DropdownMenuItem onSelect={() => welcome.open()}>
                <CompassIcon />
                <Trans>Getting started</Trans>
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onLogout}>
              <LogOutIcon />
              <Trans>Log out</Trans>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <ChangePasswordDialog open={changingPassword} onOpenChange={setChangingPassword} />
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
