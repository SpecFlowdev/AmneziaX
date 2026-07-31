import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Shell } from './components/Shell'
import { Spinner, ToastProvider } from './components/ui'
import { I18nProvider } from './i18n'
import { AuthProvider, useAuth } from './lib/auth'
import { ThemeProvider } from './lib/theme'
import { Admins } from './pages/Admins'
import { Dashboard } from './pages/Dashboard'
import { Events } from './pages/Events'
import { Hosts } from './pages/Hosts'
import { Login } from './pages/Login'
import { Nodes } from './pages/Nodes'
import { Profiles } from './pages/Profiles'
import { Settings } from './pages/Settings'
import { Squads } from './pages/Squads'
import { Subscription } from './pages/Subscription'
import { Users } from './pages/Users'
import './styles.css'

function AdminArea() {
  const { admin, ready } = useAuth()

  if (!ready) {
    return (
      <div className="center-screen">
        <Spinner />
      </div>
    )
  }
  if (!admin) return <Login />

  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Dashboard />} />
        <Route path="nodes" element={<Nodes />} />
        <Route path="hosts" element={<Hosts />} />
        <Route path="profiles" element={<Profiles />} />
        <Route path="squads" element={<Squads />} />
        <Route path="users" element={<Users />} />
        <Route path="admins" element={<Admins />} />
        <Route path="events" element={<Events />} />
        <Route path="settings" element={<Settings />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <I18nProvider>
      <ThemeProvider>
        <ToastProvider>
          <BrowserRouter>
            <Routes>
              {/* The subscriber page is public and never mounts the admin shell. */}
              <Route path="/s/:token" element={<Subscription />} />
              <Route path="/*" element={<AuthProvider><AdminArea /></AuthProvider>} />
            </Routes>
          </BrowserRouter>
        </ToastProvider>
      </ThemeProvider>
    </I18nProvider>
  </StrictMode>,
)
