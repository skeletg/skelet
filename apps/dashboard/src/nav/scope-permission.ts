// Which permission slug scopes the header's channel selector on each route - its
// own module because DashboardLayout may only export components (Fast Refresh)
export function scopePermissionForPath(pathname: string): string | undefined {
  // Covers the section/group panes (/settings/:section[/:group]) and the email paths
  if (pathname === '/settings' || pathname.startsWith('/settings/')) return 'config.view'
  if (pathname.startsWith('/pulse')) return 'emails.view'
  if (pathname.startsWith('/campaigns')) return 'campaigns.view'
  if (pathname.startsWith('/flows')) return 'flows.view'
  if (pathname.startsWith('/emails')) return 'emails.view'
  if (pathname === '/channels') return 'channels.view'
  if (pathname === '/products' || pathname.startsWith('/products/')) return 'catalog.view'
  if (pathname.startsWith('/catalog')) return 'catalog.view'
  if (pathname.startsWith('/seo')) return 'catalog.view'
  return undefined
}
