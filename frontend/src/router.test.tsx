import { render, screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { describe, expect, it } from 'vitest'
import { routes } from './router'

function renderAt(path: string) {
  render(<RouterProvider router={createMemoryRouter(routes, { initialEntries: [path] })} />)
}

describe('routes', () => {
  it('renders the home page inside the public layout', async () => {
    renderAt('/')
    expect(await screen.findByRole('heading', { name: 'Home' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: "Khan's Bike Zone" })).toBeInTheDocument()
  })

  it.each([
    ['/subscribe/confirm?token=abc', 'Confirm subscription'],
    ['/subscribe/unsubscribe?token=abc', 'Unsubscribe'],
    ['/pair?code=ABCD2345', 'Pair this phone'],
  ])('serves %s, which the backend links to', async (path, heading) => {
    renderAt(path)
    expect(await screen.findByRole('heading', { name: heading })).toBeInTheDocument()
  })

  it('shows a not-found page for unknown paths', async () => {
    renderAt('/no/such/page')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })

  it('loads the admin area on demand', async () => {
    renderAt('/admin')
    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument()
  })
})
