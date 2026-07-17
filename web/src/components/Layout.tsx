import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { getSession, setSession } from '../lib/api'

const nav = [
  { to: '/', label: 'Dashboard', exact: true },
  { to: '/drafts', label: 'Review Queue' },
  { to: '/vendors', label: 'Vendors' },
  { to: '/preferred', label: 'Preferred Vendors' },
  { to: '/properties', label: 'Properties' },
  { to: '/intake', label: 'New Request' },
]

export default function Layout() {
  const session = getSession()
  const navigate = useNavigate()

  return (
    <div className="min-h-screen flex">
      <aside className="w-60 shrink-0 bg-slate-900 text-slate-100 flex flex-col">
        <div className="px-5 py-5 border-b border-slate-700/60">
          <div className="text-lg font-semibold tracking-tight">Maintenance Hub</div>
          <div className="text-xs text-slate-400 mt-0.5">{session?.org_name}</div>
        </div>
        <nav className="flex-1 py-3">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.exact}
              className={({ isActive }) =>
                `block px-5 py-2.5 text-sm transition-colors ${
                  isActive
                    ? 'bg-slate-800 text-white border-l-2 border-sky-400'
                    : 'text-slate-300 hover:bg-slate-800/60 hover:text-white'
                }`
              }
            >
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="px-5 py-4 border-t border-slate-700/60 text-xs text-slate-400">
          <div className="truncate">{session?.email}</div>
          <button
            className="mt-2 text-slate-300 hover:text-white underline"
            onClick={() => {
              setSession(null)
              navigate('/login')
            }}
          >
            Sign out
          </button>
        </div>
      </aside>
      <main className="flex-1 min-w-0">
        <Outlet />
      </main>
    </div>
  )
}
