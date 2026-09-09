import { createBrowserRouter, Outlet, RouterProvider } from 'react-router-dom'
import { ApolloProvider } from '@apollo/client/react'
import { I18nProvider } from '@lingui/react'
import { i18n } from '@/lib/i18n'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { apolloClient } from '@/lib/apollo'
import { AuthProvider } from '@/shared/auth/auth-provider'
import { ThemeProvider } from '@/lib/theme'
import { ProtectedRoute } from '@/shared/auth/ProtectedRoute'
import { CurrencyRegistryProvider } from '@/features/currencies/CurrencyRegistryProvider'
import { DashboardLayout } from '@/components/layout/DashboardLayout'
import { PageTitleProvider } from '@/lib/page-title-provider'
import { useNavGuard } from '@/lib/nav-guard'
import { AuthShell } from '@/shared/auth/AuthShell'
import { Login } from '@/shared/auth/Login'
import { ForgotPassword } from '@/shared/auth/ForgotPassword'
import { ResetPassword } from '@/shared/auth/ResetPassword'
// Order matters - `@/nav/routes` registers core nav and `@/plugins/loader` auto-imports every plugin against the same
// registry, and both must finish before the layout's TabPanels renders the leaf table
import '@/nav/routes'
import '@/plugins/loader'
import { SignalsBanner } from '@/shared/signals/SignalsBanner'

// Everything the router hosts - the unsaved-changes navigation guard has to live INSIDE the data
// router to see navigations at all, and page titles need a location to resolve against
function RootLayout() {
  useNavGuard()
  return (
    <PageTitleProvider>
      <Outlet />
      {/* The notification toast is a custom render matching the bell dropdown's row
          design - theme comes from the themed wrapper plus app CSS variables */}
      <Toaster position="bottom-right" closeButton />
      <SignalsBanner />
    </PageTitleProvider>
  )
}

// A DATA router rather than <BrowserRouter> for one reason: useBlocker, which only data routers
// provide - it's what lets a dirty page refuse a link click or a back button before it unmounts
const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      {
        /* Public routes, outside ProtectedRoute - AuthShell is the layout, so moving
           between them swaps only the form and the brand panel stays mounted */
        element: <AuthShell />,
        children: [
          { path: '/login', element: <Login /> },
          { path: '/forgot-password', element: <ForgotPassword /> },
          { path: '/reset-password', element: <ResetPassword /> },
        ],
      },
      {
        /* One catch-all: the layout's TabPanels renders the real leaf-route table once
           PER workspace tab, so a tab switch shows a kept-alive tree instead of
           remounting the page - per-leaf wrapping (error boundary, lazy spinner,
           permission gate) moved there with it */
        path: '*',
        element: (
          <ProtectedRoute>
            <CurrencyRegistryProvider>
              <DashboardLayout />
            </CurrencyRegistryProvider>
          </ProtectedRoute>
        ),
      },
    ],
  },
])

export default function App() {
  return (
    // I18nProvider outermost so /login translates too - the active locale comes from the
    // localStorage mirror pre-auth and the roaming pref once DashboardLayout mounts
    <I18nProvider i18n={i18n}>
      <ThemeProvider>
        <AuthProvider>
          <ApolloProvider client={apolloClient}>
            <TooltipProvider>
              <RouterProvider router={router} />
            </TooltipProvider>
          </ApolloProvider>
        </AuthProvider>
      </ThemeProvider>
    </I18nProvider>
  )
}
