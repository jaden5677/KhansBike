import { createBrowserRouter, type RouteObject } from 'react-router'
import type * as Admin from './admin'
import { PublicLayout } from './layouts/PublicLayout'
import { BrandPage, BrandsPage } from './pages/BrandsPage'
import { CategoryPage } from './pages/CategoryPage'
import { ErrorPage } from './pages/ErrorPage'
import { FitmentPage } from './pages/FitmentPage'
import { HomePage } from './pages/HomePage'
import { NotFound } from './pages/NotFound'
import { PairPage } from './pages/PairPage'
import { ProductPage } from './pages/ProductPage'
import { SearchPage } from './pages/SearchPage'
import { ConfirmPage, UnsubscribePage } from './pages/SubscriptionPages'

/** A route's `lazy` loader for one page of the admin module. */
function adminPage(name: keyof typeof Admin) {
  return async () => ({ Component: (await import('./admin'))[name] })
}

// The whole site map. Some paths are fixed by the backend, which links to
// them: /subscribe/confirm and /subscribe/unsubscribe from the mailing-list
// emails, and /pair from the pairing QR code.
export const routes: RouteObject[] = [
  {
    path: '/',
    Component: PublicLayout,
    ErrorBoundary: ErrorPage,
    children: [
      { index: true, Component: HomePage },
      { path: 'c/:slug', Component: CategoryPage },
      { path: 'p/:slug', Component: ProductPage },
      { path: 'search', Component: SearchPage },
      { path: 'fitment/:wheelSize', Component: FitmentPage },
      { path: 'brands', Component: BrandsPage },
      { path: 'brands/:slug', Component: BrandPage },
      // Linked from the mailing-list confirmation email.
      { path: 'subscribe/confirm', Component: ConfirmPage },
      { path: 'subscribe/unsubscribe', Component: UnsubscribePage },
      // Linked from the pairing QR code shown in the admin.
      { path: 'pair', Component: PairPage },
      { path: '*', Component: NotFound },
    ],
  },
  // The admin area: every page is loaded on demand from one module, so
  // customers never download the admin code. Sign-in sits outside the
  // guarded layout; everything under it requires a signed-in admin.
  { path: '/admin/login', ErrorBoundary: ErrorPage, lazy: adminPage('LoginPage') },
  {
    path: '/admin',
    ErrorBoundary: ErrorPage,
    lazy: adminPage('AdminLayout'),
    children: [
      { index: true, lazy: adminPage('DashboardPage') },
      { path: 'products', lazy: adminPage('ProductsPage') },
      { path: 'products/new', lazy: adminPage('ProductEditorPage') },
      { path: 'products/:id', lazy: adminPage('ProductEditorPage') },
      { path: 'categories', lazy: adminPage('CategoriesPage') },
      { path: 'categories/:id', lazy: adminPage('CategoryEditPage') },
      { path: 'attributes', lazy: adminPage('AttributesPage') },
      { path: 'attributes/:id', lazy: adminPage('AttributeEditPage') },
      { path: 'brands-suppliers', lazy: adminPage('BrandsSuppliersPage') },
      { path: '*', Component: NotFound },
    ],
  },
]

export const router = createBrowserRouter(routes)
