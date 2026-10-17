import { useQuery } from '@apollo/client/react'
import { Badge } from '@/components/ui/badge'
import { MeDocument } from '@/shared/auth/Me.gql'
import { Panel, WidgetSection } from './primitives'

export function AccountWidget() {
  const { data, loading, error } = useQuery(MeDocument, { fetchPolicy: 'cache-first' })
  const me = data?.me

  return (
    <WidgetSection
      title="Your account"
      description="Who you're signed in as and what you can do."
      to="/preferences?tab=account"
      toLabel="Preferences"
      loading={loading && !me}
      error={error ?? null}
      empty={!loading && !me}
    >
      <div className="grid gap-4 lg:grid-cols-2">
        <Panel title="Signed in">
          <dl className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Email</dt>
            <dd className="truncate">{me?.email}</dd>
            <dt className="text-muted-foreground">Name</dt>
            <dd>
              {me?.firstName} {me?.lastName}
            </dd>
            <dt className="self-start text-muted-foreground">Roles</dt>
            <dd>
              {!me?.roles.length ? (
                <span className="text-muted-foreground">none</span>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {me.roles.map((role) => (
                    <Badge key={role.id} variant="secondary">
                      {role.name}
                    </Badge>
                  ))}
                </div>
              )}
            </dd>
          </dl>
        </Panel>

        <Panel title="Effective permissions">
          {me?.isSuperuser ? (
            <p className="text-sm text-muted-foreground">
              Superuser - permission checks are bypassed for this account.
            </p>
          ) : !me?.permissions.length ? (
            <p className="text-sm text-muted-foreground">No permissions granted.</p>
          ) : (
            <ul className="flex flex-wrap gap-2">
              {me.permissions.map((p) => (
                <li key={p.id} className="rounded border bg-muted px-2 py-0.5 font-mono text-xs">
                  {p.slug}
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </WidgetSection>
  )
}
