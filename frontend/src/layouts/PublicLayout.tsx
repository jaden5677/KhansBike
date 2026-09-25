import { Link, Outlet } from 'react-router'

/** The frame around every customer page: header, page content, footer. */
export function PublicLayout() {
  return (
    <div className="page">
      <header className="site-header">
        <Link to="/" className="brand">
          Khan's Bike Zone
        </Link>
        <nav aria-label="Main">
          <Link to="/brands">Brands</Link>
        </nav>
      </header>
      <main className="content">
        <Outlet />
      </main>
      <footer className="site-footer">
        <p>Bicycles, parts and accessories in Trinidad &amp; Tobago.</p>
      </footer>
    </div>
  )
}
