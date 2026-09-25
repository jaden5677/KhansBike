import { Link, Outlet } from 'react-router'

/** The frame around every admin screen. Sign-in protection arrives in step 4. */
export function AdminLayout() {
  return (
    <div className="page">
      <header className="site-header">
        <Link to="/admin" className="brand">
          Admin
        </Link>
        <nav aria-label="Admin">
          <Link to="/">View site</Link>
        </nav>
      </header>
      <main className="content">
        <Outlet />
      </main>
    </div>
  )
}
