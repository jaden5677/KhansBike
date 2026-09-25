import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderApp } from './test/renderApp'

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

  it('loads the admin area on demand', async () => {
    renderApp('/admin')
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument()
  })
})
