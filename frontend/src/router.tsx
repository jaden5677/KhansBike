import { createBrowserRouter, type RouteObject } from 'react-router'
import { PublicLayout } from './layouts/PublicLayout'
import { BrandPage, BrandsPage } from './pages/BrandsPage'
import { CategoryPage } from './pages/CategoryPage'
import { ErrorPage } from './pages/ErrorPage'
import { FitmentPage } from './pages/FitmentPage'
import { HomePage } from './pages/HomePage'
import { NotFound } from './pages/NotFound'
import { Placeholder } from './pages/Placeholder'
import { ProductPage } from './pages/ProductPage'
import { SearchPage } from './pages/SearchPage'

// The whole site map. The remaining placeholder pages are replaced step by
// step; their paths are already final because the backend links to them
// (confirmation emails and phone pairing).
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
      { path: 'subscribe/confirm', element: <Placeholder title="Confirm subscription" /> },
      { path: 'subscribe/unsubscribe', element: <Placeholder title="Unsubscribe" /> },
      // Linked from the pairing QR code shown in the admin.
      { path: 'pair', element: <Placeholder title="Pair this phone" /> },
      { path: '*', Component: NotFound },
    ],
  },
  {
    // Loaded on demand, so customers never download the admin code.
    path: '/admin',
    ErrorBoundary: ErrorPage,
    lazy: async () => ({ Component: (await import('./admin/AdminLayout')).AdminLayout }),
    children: [
      { index: true, element: <Placeholder title="Dashboard" /> },
      { path: 'login', element: <Placeholder title="Sign in" /> },
      { path: '*', Component: NotFound },
    ],
  },
]

export const router = createBrowserRouter(routes)
