import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { BugIcon, CheckIcon, ChevronsUpDownIcon, FlaskConicalIcon, Loader2Icon } from 'lucide-react'

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
import { setAuthStoredState, useAuth } from './auth'
import { DevAdminsDocument, DevImpersonateDocument } from './Dev.gql'
import { DevInspector } from './DevInspector'

// Dev-only identity switcher - hop between admin accounts to test permission and channel-scope
// behaviour without logging in and out - Lives in the sidebar footer, following the NavUser
// pattern, and renders nothing outside development: AppSidebar mounts it behind
// import.meta.env.DEV, and the backend hard-errors both dev operations unless
// APP_ENV=development, so a production bundle can't expose it by mistake
//
// Switching writes the impersonated session into the stored AuthState, the same shape Login
// persists, and hard-reloads:
export function DevAdminSwitcher() {
  const { auth } = useAuth()
  const { isMobile } = useSidebar()
  const admin = auth?.admin
  const { data } = useQuery(DevAdminsDocument, {
    // The backend errors outside dev mode - treat that as "feature
    // off" and render nothing rather than surfacing an error
    errorPolicy: 'ignore',
    fetchPolicy: 'cache-first',
  })
  const [impersonate] = useMutation(DevImpersonateDocument)
  const [switchingTo, setSwitchingTo] = useState<string | null>(null)
  const [inspectorOpen, setInspectorOpen] = useState(false)

  const admins = data?.devAdmins ?? []
  if (admins.length === 0) return null

  async function onPick(adminId: string) {
    setSwitchingTo(adminId)
    try {
      const res = await impersonate({ variables: { adminId } })
      const payload = res.data?.devImpersonate
      if (!payload) return
      setAuthStoredState({
        accessToken: payload.accessToken,
        refreshToken: payload.refreshToken,
        admin: payload.admin,
      })
      window.location.reload()
    } catch {
      setSwitchingTo(null)
    }
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              tooltip="Dev: switch admin identity"
              className="border border-dashed border-warning/50 data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
            >
              <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-warning-soft">
                <FlaskConicalIcon className="size-4 text-warning" />
              </div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-mono text-[10px] font-semibold uppercase tracking-wider text-warning">
                  dev mode
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  Switch identity
                </span>
              </div>
              <ChevronsUpDownIcon className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-72 rounded-lg"
            side={isMobile ? 'bottom' : 'right'}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="text-xs">
              Switch admin identity
              <span className="block font-normal text-muted-foreground">
                Test users - create with <code className="font-mono">bin/cli admin create</code>
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {admins.map((a) => {
              const current = a.email === admin?.email
              const roleSummary = a.isSuperuser
                ? 'superuser'
                : a.roles.map((r) => r.slug).join(', ') || 'no roles'
              return (
                <DropdownMenuItem
                  key={a.id}
                  disabled={current || !a.isActive || switchingTo !== null}
                  onSelect={(e) => {
                    e.preventDefault()
                    void onPick(a.id)
                  }}
                  className="grid gap-0.5"
                >
                  <span className="flex items-center gap-1.5 text-sm">
                    {switchingTo === a.id ? (
                      <Loader2Icon className="size-3 animate-spin" />
                    ) : current ? (
                      <CheckIcon className="size-3 text-success" />
                    ) : (
                      <span className="size-3" />
                    )}
                    {a.email}
                    {!a.isActive && (
                      <span className="text-[10px] uppercase text-muted-foreground">inactive</span>
                    )}
                  </span>
                  <span className="pl-[18px] text-xs text-muted-foreground">{roleSummary}</span>
                </DropdownMenuItem>
              )
            })}
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setInspectorOpen(true)}>
              <BugIcon className="size-3.5" />
              Open dev inspector
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
      <DevInspector open={inspectorOpen} onOpenChange={setInspectorOpen} />
    </SidebarMenu>
  )
}
