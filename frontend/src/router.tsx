import { createBrowserRouter, type RouteObject } from 'react-router'
import { PublicLayout } from './layouts/PublicLayout'
import { ErrorPage } from './pages/ErrorPage'
import { NotFound } from './pages/NotFound'
import { Placeholder } from './pages/Placeholder'

// The whole site map. Placeholder pages are replaced by real ones step by
// step; the paths are already final because the backend links to some of
// them (confirmation emails and phone pairing).
export const routes: RouteObject[] = [
  {
    path: '/',
    Component: PublicLayout,
    ErrorBoundary: ErrorPage,
    children: [
      { index: true, element: <Placeholder title="Home" /> },
      { path: 'c/:slug', element: <Placeholder title="Category" /> },
      { path: 'p/:slug', element: <Placeholder title="Product" /> },
      { path: 'search', element: <Placeholder title="Search" /> },
      { path: 'fitment/:wheelSize', element: <Placeholder title="Fitment" /> },
      { path: 'brands', element: <Placeholder title="Brands" /> },
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
