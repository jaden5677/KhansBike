import { Link, Navigate, NavLink, Outlet, useLocation } from 'react-router'
import { useLogout, useSession } from '../auth/session'
import { ErrorText } from '../components/ErrorText'
import styles from './admin.module.css'

/**
 * The frame around every admin screen, and its guard: with nobody signed in
 * it sends the visitor to the sign-in page, remembering where they wanted
 * to go. The session query is the single source of truth, so this also
 * reacts when any request finds the sign-in has expired.
 */
export function AdminLayout() {
  const session = useSession()
  const logout = useLogout()
  const location = useLocation()

  if (session.isPending) {
    return (
      <p role="status" className="content muted">
        Loading…
      </p>
    )
  }
  if (session.isError) {
    return (
      <div className="content">
        <ErrorText error={session.error} />
        <button type="button" onClick={() => session.refetch()}>
          Try again
        </button>
      </div>
    )
  }
  if (!session.data) {
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/admin/login?next=${next}`} replace />
  }

  const me = session.data
  return (
    <div className="page">
      <header className="site-header">
        <Link to="/admin" className="brand">
          Khan's Bike Zone admin
        </Link>
        <nav aria-label="Admin" className={styles.nav}>
          <NavLink to="/admin" end>
            Dashboard
          </NavLink>
          <NavLink to="/admin/products">Products</NavLink>
          <a href="/" target="_blank" rel="noreferrer">
            View site
          </a>
        </nav>
        <div className={styles.account}>
          <span className="muted">{me.kind === 'device' ? `${me.user.displayName} (this phone)` : me.user.displayName}</span>
          <button type="button" onClick={() => logout.mutate(me)} disabled={logout.isPending}>
            {me.kind === 'device' ? 'Unpair' : 'Sign out'}
          </button>
        </div>
      </header>
      <main className="content">
        <Outlet />
      </main>
    </div>
  )
}
