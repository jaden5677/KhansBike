import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { productPage, riderDetail, searchGroups, tyreFacets, tyresCategory } from '../test/fixtures'
import { renderApp } from '../test/renderApp'

const categoryAPI = {
  '/categories/tyres': tyresCategory,
  '/categories/tyres/facets': { items: tyreFacets },
  '/products': productPage,
}

describe('category page', () => {
  it('lists products with prices, or "Ask in store" when the price is private', async () => {
    renderApp('/c/tyres', categoryAPI)
    expect(await screen.findByRole('heading', { name: 'Tyres' })).toBeInTheDocument()
    expect(await screen.findByText('Rider 20')).toBeInTheDocument()
    expect(screen.getByText('$85.00')).toBeInTheDocument()
    expect(screen.getByText('Ask in store')).toBeInTheDocument()
    expect(screen.getByText('2 products')).toBeInTheDocument()
    // Breadcrumbs: the ancestors, then the current category.
    const crumbs = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(within(crumbs).getByRole('link', { name: 'Wheels' })).toHaveAttribute('href', '/c/wheels')
  })

  it('puts a ticked filter in the URL and sends it to the API', async () => {
    const { router, requests } = renderApp('/c/tyres', categoryAPI)
    fireEvent.click(await screen.findByRole('checkbox', { name: /Red/ }))
    await waitFor(() => expect(router.state.location.search).toBe('?attr.colour=red'))
    await waitFor(() =>
      expect(requests).toContain('/api/v1/products?attr.colour=red&category=tyres&limit=24'),
    )
    expect(requests).toContain('/api/v1/categories/tyres/facets?attr.colour=red')
    expect(screen.getByRole('checkbox', { name: /Red/ })).toBeChecked()
  })

  it('applies a numeric range', async () => {
    const { router } = renderApp('/c/tyres', categoryAPI)
    fireEvent.change(await screen.findByRole('spinbutton', { name: 'Width from' }), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))
    await waitFor(() => expect(router.state.location.search).toBe('?attr.width_min=2'))
  })

  it('changes the sort order', async () => {
    const { router } = renderApp('/c/tyres', categoryAPI)
    fireEvent.change(await screen.findByRole('combobox', { name: /Sort by/ }), { target: { value: 'name' } })
    await waitFor(() => expect(router.state.location.search).toBe('?sort=name'))
  })
})

describe('product page', () => {
  it('shows the default variant, and switches variant through the URL', async () => {
    const { router } = renderApp('/p/rider-20', { '/products/rider-20': riderDetail })
    expect(await screen.findByRole('heading', { name: 'Rider 20' })).toBeInTheDocument()
    expect(screen.getByText('$85.00')).toBeInTheDocument()
    expect(screen.getByText(/RID-20-RED/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Black' }))
    await waitFor(() => expect(router.state.location.search).toBe('?variant=v-black'))
    expect(screen.getByRole('button', { name: 'Black' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('Ask in store')).toBeInTheDocument()
    expect(screen.getByText('Out of stock')).toBeInTheDocument()
  })

  it('lists specifications and links to parts of the same wheel size', async () => {
    renderApp('/p/rider-20', { '/products/rider-20': riderDetail })
    expect(await screen.findByText('20 in')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'More parts that fit 20 in' })).toHaveAttribute('href', '/fitment/20')
  })

  it('shows the not-found page for an unknown product', async () => {
    renderApp('/p/nope')
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument()
  })
})

describe('search', () => {
  it('groups results by category and links to the full listing', async () => {
    renderApp('/search?q=rider', { '/search': { groups: searchGroups } })
    expect(await screen.findByRole('heading', { name: /Tyres/ })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'See all 5 in Tyres' })).toHaveAttribute('href', '/c/tyres?q=rider')
  })

  it('suggests product names while typing', async () => {
    renderApp('/search', { '/search/suggest': { items: [{ slug: 'rider-20', name: 'Rider 20' }] } })
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search products' }), { target: { value: 'rid' } })
    const link = await screen.findByRole('link', { name: 'Rider 20' })
    expect(link).toHaveAttribute('href', '/p/rider-20')
  })

  it('opens the results page on submit', async () => {
    const { router } = renderApp('/', { '/categories': { items: [] }, '/products': { items: [] } })
    fireEvent.change(screen.getByRole('searchbox', { name: 'Search products' }), { target: { value: 'tube 26' } })
    fireEvent.submit(screen.getByRole('search'))
    await waitFor(() => expect(router.state.location.pathname + router.state.location.search).toBe('/search?q=tube%2026'))
  })
})
