import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { hasDeviceToken } from '../auth/deviceToken'
import { problem, Reply, renderApp } from '../test/renderApp'

// The home page needs these to render; the footer is on every public page.
const home = { '/categories': { items: [] }, '/products': { items: [] } }

describe('mailing list signup', () => {
  it('subscribes from the footer', async () => {
    const { calls } = renderApp('/', { ...home, '/subscribers': new Reply(202, { status: 'ok' }) })
    const footer = screen.getByRole('contentinfo')
    fireEvent.change(within(footer).getByLabelText('Email'), { target: { value: ' ana@example.com ' } })
    fireEvent.click(within(footer).getByRole('button', { name: 'Subscribe' }))
    expect(await within(footer).findByText(/Check your inbox/)).toBeInTheDocument()
    expect(calls.find((c) => c.path === '/subscribers')?.body).toEqual({ email: 'ana@example.com', source: 'footer' })
  })

  it("shows the server's message beside the field", async () => {
    renderApp('/', {
      ...home,
      '/subscribers': problem(422, 'Validation failed', [{ field: 'email', message: 'is not a valid email address' }]),
    })
    const footer = screen.getByRole('contentinfo')
    fireEvent.change(within(footer).getByLabelText('Email'), { target: { value: 'nope' } })
    fireEvent.click(within(footer).getByRole('button', { name: 'Subscribe' }))
    expect(await within(footer).findByText('Email is not a valid email address')).toBeInTheDocument()
    expect(within(footer).getByLabelText('Email')).toHaveAttribute('aria-invalid', 'true')
  })
})

describe('confirmation page', () => {
  it('confirms once as soon as it opens, even though effects run twice in StrictMode', async () => {
    const { calls } = renderApp('/subscribe/confirm?token=tok-1', { '/subscribers/confirm': new Reply(204) })
    expect(await screen.findByText(/You're subscribed/)).toBeInTheDocument()
    const confirms = calls.filter((c) => c.path === '/subscribers/confirm')
    expect(confirms).toHaveLength(1)
    expect(confirms[0].body).toEqual({ token: 'tok-1' })
  })

  it('offers to sign up again when the link is spent', async () => {
    renderApp('/subscribe/confirm?token=old', {
      '/subscribers/confirm': problem(404, 'the link is invalid or has expired'),
    })
    expect(await screen.findByText(/expired or was already used/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Subscribe' }).length).toBeGreaterThan(0)
  })
})

describe('unsubscribe page', () => {
  it('waits for a button press, so link scanners cannot unsubscribe anyone', async () => {
    const { calls } = renderApp('/subscribe/unsubscribe?token=tok-2', { '/subscribers/unsubscribe': new Reply(204) })
    const button = await screen.findByRole('button', { name: 'Unsubscribe' })
    expect(calls.some((c) => c.path === '/subscribers/unsubscribe')).toBe(false)
    fireEvent.click(button)
    expect(await screen.findByText(/removed from our mailing list/)).toBeInTheDocument()
    expect(calls.find((c) => c.path === '/subscribers/unsubscribe')?.body).toEqual({ token: 'tok-2' })
  })
})

describe('pairing page', () => {
  const paired = { token: 'device-token-1', device: { id: 'd1', name: 'Shop phone', createdAt: '2026-09-25T00:00:00Z' } }

  it('pairs with the code from the QR link, then uses the token for admin requests', async () => {
    const { router, calls } = renderApp('/pair?code=ABCD-2345', { '/auth/devices': new Reply(201, paired) })
    expect(screen.getByLabelText('Pairing code')).toHaveValue('ABCD-2345')
    fireEvent.change(screen.getByLabelText('Name for this phone'), { target: { value: 'Shop phone' } })
    fireEvent.click(screen.getByRole('button', { name: 'Pair phone' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/admin'))
    expect(calls.find((c) => c.path === '/auth/devices')?.body).toEqual({ code: 'ABCD-2345', name: 'Shop phone' })
    expect(hasDeviceToken()).toBe(true)
  })

  it('explains a code that is wrong or expired', async () => {
    renderApp('/pair?code=WRONG', {
      '/auth/devices': problem(401, 'the pairing code is invalid, expired or already used'),
    })
    fireEvent.change(screen.getByLabelText('Name for this phone'), { target: { value: 'Shop phone' } })
    fireEvent.click(screen.getByRole('button', { name: 'Pair phone' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('invalid, expired or already used')
  })
})
