import { useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { useI18n, type Lang } from '../i18n'
import { useAuth } from '../lib/auth'
import { useTheme } from '../lib/theme'
import { Icon } from './icons'
import { Brand } from './Brand'

interface NavEntry {
  to: string
  label: string
  icon: string
  ownerOnly?: boolean
}

export function Shell() {
  const { t, lang, setLang } = useI18n()
  const { admin, logout, isOwner } = useAuth()
  const { theme, setTheme } = useTheme()
  const [menuOpen, setMenuOpen] = useState(false)
  const location = useLocation()

  const groups: { title: string; items: NavEntry[] }[] = [
    {
      title: t.nav.overview,
      items: [{ to: '/', label: t.nav.dashboard, icon: 'dashboard' }],
    },
    {
      title: t.nav.infrastructure,
      items: [
        { to: '/nodes', label: t.nav.nodes, icon: 'server' },
        { to: '/hosts', label: t.nav.hosts, icon: 'globe' },
        { to: '/profiles', label: t.nav.profiles, icon: 'code' },
      ],
    },
    {
      title: t.nav.access,
      items: [
        { to: '/squads', label: t.nav.squads, icon: 'layers' },
        { to: '/users', label: t.nav.users, icon: 'users' },
      ],
    },
    {
      title: t.nav.system,
      items: [
        { to: '/admins', label: t.nav.admins, icon: 'shield', ownerOnly: true },
        { to: '/events', label: t.nav.events, icon: 'activity' },
        { to: '/settings', label: t.nav.settings, icon: 'settings' },
      ],
    },
  ]

  const currentTitle =
    groups.flatMap((g) => g.items).find((i) => i.to === location.pathname)?.label ??
    t.nav.dashboard

  return (
    <div className="shell">
      <aside className={`sidebar${menuOpen ? ' open' : ''}`}>
        <div className="brand">
          <Brand />
        </div>

        {groups.map((group) => {
          const items = group.items.filter((i) => !i.ownerOnly || isOwner)
          if (items.length === 0) return null
          return (
            <div key={group.title}>
              <div className="nav-section">{group.title}</div>
              {items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === '/'}
                  className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
                  onClick={() => setMenuOpen(false)}
                >
                  <Icon name={item.icon} size={17} />
                  {item.label}
                </NavLink>
              ))}
            </div>
          )
        })}

        <div style={{ flex: 1 }} />

        <div className="nav-section">{admin?.username}</div>
        <button className="nav-item" onClick={logout} style={{ background: 'none' }}>
          <Icon name="logout" size={17} />
          {t.common.logout}
        </button>
      </aside>

      <div className="main">
        <header className="topbar">
          <button
            className="btn-ghost btn-icon"
            onClick={() => setMenuOpen((v) => !v)}
            style={{ display: 'none' }}
            data-mobile-menu
            aria-label="Menu"
          >
            <Icon name="menu" />
          </button>
          <h1>{currentTitle}</h1>
          <div className="topbar-spacer" />

          <div className="tabs" role="group" aria-label="Language">
            {(['ru', 'en'] as Lang[]).map((l) => (
              <button
                key={l}
                className={`tab${lang === l ? ' active' : ''}`}
                onClick={() => setLang(l)}
              >
                {l === 'ru' ? 'РУС' : 'ENG'}
              </button>
            ))}
          </div>

          <button
            className="btn-ghost btn-icon"
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            title={t.common.theme}
            aria-label={t.common.theme}
          >
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} size={17} />
          </button>
        </header>

        <Outlet />
      </div>
    </div>
  )
}
