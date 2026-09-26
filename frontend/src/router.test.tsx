import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { problem, renderApp } from './test/renderApp'

describe('routes', () => {
  it('renders the home page inside the public layout', async () => {
    renderApp('/', { '/categories': { items: [] }, '/products': { items: [] } })
    expect(await screen.findByRole('heading', { name: 'Bikes, parts and accessories' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: "Khan's Bike Zone" })).toBeInTheDocument()
    expect(screen.getByRole('search')).toBeInTheDocument()
  })

  it.each([
    ['/subscribe/confirm?token=abc', 'Mailing list'],
    ['/subscribe/unsubscribe?token=abc', 'Unsubscribe'],
    ['/pair?code=ABCD2345', 'Pair this phone'],
  ])('serves %s, which the backend links to', async (path, heading) => {
    renderApp(path)
    expect(await screen.findByRole('heading', { name: heading })).toBeInTheDocument()
  })

  it('shows a not-found page for unknown paths', async () => {
    renderApp('/no/such/page')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })

  it('sends visitors who are not signed in from the admin to the sign-in page', async () => {
    const { router } = renderApp('/admin/products?status=draft', { '/auth/session': problem(401, 'sign in') })
    expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/admin/login')
    expect(router.state.location.search).toBe('?next=%2Fadmin%2Fproducts%3Fstatus%3Ddraft')
  })
})
