import { Link, Outlet, ScrollRestoration } from 'react-router'
import { SearchBox } from '../components/SearchBox'
import { SubscribeForm } from '../components/SubscribeForm'

/** The frame around every customer page: header, page content, footer. */
export function PublicLayout() {
  return (
    <div className="page">
      <header className="site-header">
        <Link to="/" className="brand">
          Khan's Bike Zone
        </Link>
        <SearchBox />
        <nav aria-label="Main">
          <Link to="/brands">Brands</Link>
        </nav>
      </header>
      <main className="content">
        <Outlet />
      </main>
      <footer className="site-footer">
        <SubscribeForm source="footer" />
        <p>Bicycles, parts and accessories in Trinidad &amp; Tobago.</p>
      </footer>
      {/* New pages start at the top; Back returns to where you were. */}
      <ScrollRestoration />
    </div>
  )
}
