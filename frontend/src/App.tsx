import { lazy, useEffect } from 'react'
import {
  Navigate,
  Route,
  Routes,
  useHref,
  useNavigate,
  type NavigateOptions,
} from 'react-router-dom'
import { RouterProvider } from 'react-aria-components'
import { setUnauthorizedHandler } from './lib/api'
import { RequireAuth } from './components/RequireAuth'
import { AppLayout } from './components/AppLayout'
import LoginPage from './pages/Login'

// Route-level code splitting: the authed pages load on demand behind the
// AppLayout Suspense boundary, keeping the initial (login) bundle lean.
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Groups = lazy(() => import('./pages/Groups'))
const GroupHistory = lazy(() => import('./pages/groups/GroupHistory'))
const Logs = lazy(() => import('./pages/Logs'))
const Masters = lazy(() => import('./pages/Masters'))
const Rules = lazy(() => import('./pages/Rules'))
const Intelligence = lazy(() => import('./pages/Intelligence'))
const Connection = lazy(() => import('./pages/Connection'))

// Let react-aria's RouterProvider carry react-router's navigate options so
// HeroUI links/sidebar items type-check with `routerOptions` (documented
// react-aria ↔ react-router integration).
declare module 'react-aria-components' {
  interface RouterConfig {
    routerOptions: NavigateOptions
  }
}

export default function App() {
  const navigate = useNavigate()

  // A 401 from any protected call (api.ts) bounces the user to /login.
  useEffect(() => {
    setUnauthorizedHandler(() => navigate('/login', { replace: true }))
    return () => setUnauthorizedHandler(null)
  }, [navigate])

  return (
    <RouterProvider navigate={navigate} useHref={useHref}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth><AppLayout /></RequireAuth>}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/groups" element={<Groups />} />
          <Route path="/groups/:groupId/history" element={<GroupHistory />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/masters" element={<Masters />} />
          <Route path="/rules" element={<Rules />} />
          <Route path="/intelligence" element={<Intelligence />} />
          <Route path="/connection" element={<Connection />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </RouterProvider>
  )
}
