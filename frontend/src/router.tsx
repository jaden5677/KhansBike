import { createBrowserRouter, type RouteObject } from 'react-router'
import { PublicLayout } from './layouts/PublicLayout'
import { BrandPage, BrandsPage } from './pages/BrandsPage'
import { CategoryPage } from './pages/CategoryPage'
import { ErrorPage } from './pages/ErrorPage'
import { FitmentPage } from './pages/FitmentPage'
import { HomePage } from './pages/HomePage'
import { NotFound } from './pages/NotFound'
import { PairPage } from './pages/PairPage'
import { Placeholder } from './pages/Placeholder'
import { ProductPage } from './pages/ProductPage'
import { SearchPage } from './pages/SearchPage'
import { ConfirmPage, UnsubscribePage } from './pages/SubscriptionPages'

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
