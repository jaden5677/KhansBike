import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { Image } from '../api/types'
import { Picture } from './Picture'
import { formatMoney, Price } from './Price'

describe('Price', () => {
  it('formats Trinidad dollars and US dollars', () => {
    expect(formatMoney({ amount: '1250.00', currency: 'TTD' })).toBe('$1,250.00')
    expect(formatMoney({ amount: '12.5', currency: 'USD' })).toBe('US$12.50')
  })

  it('says "Ask in store" when the price is not public', () => {
    render(<Price money={null} />)
    expect(screen.getByText('Ask in store')).toBeInTheDocument()
  })
})

describe('Picture', () => {
  const image: Image = {
    width: 1600,
    height: 1200,
    dominantHex: '#aa3322',
    sources: [
      { url: '/media/r/a/640.webp', width: 640, height: 480, format: 'webp' },
      { url: '/media/r/a/320.jpeg', width: 320, height: 240, format: 'jpeg' },
      { url: '/media/r/a/640.jpeg', width: 640, height: 480, format: 'jpeg' },
      { url: '/media/r/a/320.webp', width: 320, height: 240, format: 'webp' },
    ],
  }

  it('offers WebP with a JPEG fallback, smallest first', () => {
    const { container } = render(<Picture image={image} alt="Red tyre" sizes="50vw" />)
    const source = container.querySelector('source')!
    expect(source).toHaveAttribute('type', 'image/webp')
    expect(source).toHaveAttribute('srcset', '/media/r/a/320.webp 320w, /media/r/a/640.webp 640w')
    const img = screen.getByRole('img', { name: 'Red tyre' })
    expect(img).toHaveAttribute('src', '/media/r/a/640.jpeg')
    expect(img).toHaveAttribute('srcset', '/media/r/a/320.jpeg 320w, /media/r/a/640.jpeg 640w')
    expect(img).toHaveAttribute('sizes', '50vw')
    expect(img).toHaveAttribute('loading', 'lazy')
  })

  it('reserves space and shows the dominant colour while loading', () => {
    render(<Picture image={image} alt="Red tyre" sizes="50vw" />)
    const img = screen.getByRole('img')
    expect(img).toHaveAttribute('width', '1600')
    expect(img).toHaveAttribute('height', '1200')
    expect(img.style.backgroundColor).toBe('rgb(170, 51, 34)')
  })
})
