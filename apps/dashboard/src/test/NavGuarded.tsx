import { Outlet } from 'react-router-dom'
import { useNavGuard } from '@/lib/nav-guard'

// The layout route App.tsx gives every page: the leave-page guard over whatever is routed below it
export function NavGuarded() {
  useNavGuard()
  return <Outlet />
}
