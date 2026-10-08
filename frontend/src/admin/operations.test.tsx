import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { FormField, ImportBatch, ImportRow } from '../api/adminTypes'
import type { Session } from '../api/types'
import { QRCode } from '../components/QRCode'
import { formatAttributeValue } from '../lib/attributes'
import { changedFields, describeAction, entityLink } from '../lib/audit'
import { problem, Reply, renderApp, type Call, type Responder } from '../test/renderApp'
import { formatCode } from './DevicesPage'
import { countsSummary } from './ImportsPage'

const session: Session = {
  user: { id: 'u1', email: 'owner@example.com', displayName: 'Owner', role: 'admin' },
  kind: 'admin',
  csrfToken: 'csrf-1',
}

const find = (calls: Call[], method: string, path: string) => calls.filter((c) => c.method === method && c.path === path)

function api(extra: Record<string, Responder> = {}): Record<string, Responder> {
  return { '/auth/session': session, ...extra }
}

describe('import review', () => {
  const batch: ImportBatch = {
    id: 'b1',
    filename: 'prices.xlsx',
    status: 'dry_run',
    counts: { pending: 1, accept: 1 },
    createdAt: '2026-10-01T10:00:00Z',
  }
  const errorRow: ImportRow = {
    id: 'r1',
    sheet: 'Tubes',
    row: 4,
    raw: { Item: '26 x 1.95 tube', Retail: 'abc' },
    proposed: { categoryId: 'c1', name: '26 x 1.95 tube', status: 'active', sku: 'TUBE-26', stockStatus: 'in_stock' },
    issues: [{ code: 'bad_price', severity: 'error', field: 'retail_ttd', message: '"abc" is not a price' }],
    decision: 'pending',
  }
  const goodRow: ImportRow = {
    ...errorRow,
    id: 'r2',
    row: 5,
    raw: { Item: '20 x 1.95 tube', Retail: '30' },
    proposed: { ...errorRow.proposed, name: '20 x 1.95 tube', sku: 'TUBE-20', prices: { retail_ttd: '30.00' } },
    issues: [],
    decision: 'accept',
  }

  it('uploads a workbook and opens its review', async () => {
    const { calls, router } = renderApp(
      '/admin/imports',
      api({
        '/admin/imports': (_u: URL, c: Call) => (c.method === 'POST' ? new Reply(201, batch) : { items: [] }),
        '/admin/imports/b1': batch,
        '/admin/imports/b1/rows': { items: [] },
      }),
    )
    const input = await screen.findByLabelText('Workbook')
    fireEvent.change(input, { target: { files: [new File(['x'], 'prices.xlsx')] } })
    fireEvent.click(screen.getByRole('button', { name: 'Upload and review' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/imports/b1'))
    expect(find(calls, 'POST', '/admin/imports')).toHaveLength(1)
  })

  it('offers only the decisions the server allows, and updates the row in place', async () => {
    const { calls } = renderApp(
      '/admin/imports/b1',
      api({
        '/admin/imports/b1': batch,
        '/admin/imports/b1/rows': {
          items: [
            errorRow,
            goodRow,
            { ...goodRow, id: 'r6', row: 6, decision: 'merge', proposed: { ...goodRow.proposed, targetVariantId: 'v9' } },
          ],
        },
        '/admin/imports/b1/rows/r1': { ...errorRow, decision: 'skip' },
      }),
    )
    const rowGroup = await screen.findByRole('group', { name: 'Decision for Tubes row 4' })
    // A row with errors cannot be accepted, and matches nothing to merge into.
    expect(within(rowGroup).getAllByRole('button').map((b) => b.textContent)).toEqual(['Skip'])
    // A row matching an existing variant can be merged or skipped, never added again.
    const matched = screen.getByRole('group', { name: 'Decision for Tubes row 6' })
    expect(within(matched).getAllByRole('button').map((b) => b.textContent)).toEqual(['Merge', 'Skip'])
    expect(screen.getByText('"abc" is not a price')).toBeInTheDocument()
    // Applying is blocked while a row waits.
    expect(screen.getByRole('button', { name: 'Apply to the catalogue' })).toBeDisabled()

    const rowLoads = find(calls, 'GET', '/admin/imports/b1/rows').length
    fireEvent.click(within(rowGroup).getByRole('button', { name: 'Skip' }))
    await waitFor(() => expect(within(rowGroup).getByRole('button', { name: 'Skip' })).toHaveAttribute('aria-pressed', 'true'))
    expect(find(calls, 'PUT', '/admin/imports/b1/rows/r1')[0].body).toEqual({ decision: 'skip' })
    expect(find(calls, 'GET', '/admin/imports/b1/rows')).toHaveLength(rowLoads) // the list was not reloaded
    await waitFor(() => expect(find(calls, 'GET', '/admin/imports/b1').length).toBeGreaterThan(1)) // counts were
  })

  it('skips every waiting row in one go', async () => {
    const { calls } = renderApp(
      '/admin/imports/b1',
      api({
        '/admin/imports/b1': { ...batch, counts: { pending: 2 } },
        '/admin/imports/b1/rows': (url: URL) =>
          url.searchParams.get('decision') === 'pending' ? { items: [errorRow, { ...errorRow, id: 'r3' }] } : { items: [errorRow] },
        '/admin/imports/b1/rows/r1': { ...errorRow, decision: 'skip' },
        '/admin/imports/b1/rows/r3': { ...errorRow, id: 'r3', decision: 'skip' },
      }),
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Skip all 2 waiting rows' }))
    await waitFor(() => expect(find(calls, 'PUT', '/admin/imports/b1/rows/r3')).toHaveLength(1))
    expect(find(calls, 'PUT', '/admin/imports/b1/rows/r1')[0].body).toEqual({ decision: 'skip' })
  })

  it('applies the batch with the chosen price visibility', async () => {
    const ready = { ...batch, counts: { accept: 2 } }
    const { calls } = renderApp(
      '/admin/imports/b1',
      api({
        '/admin/imports/b1': ready,
        '/admin/imports/b1/rows': { items: [goodRow] },
        '/admin/imports/b1/commit': { ...ready, status: 'committed' },
      }),
    )
    fireEvent.click(await screen.findByRole('checkbox', { name: /Show the retail price/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Apply to the catalogue' }))
    await waitFor(() => expect(find(calls, 'POST', '/admin/imports/b1/commit')).toHaveLength(1))
    expect(find(calls, 'POST', '/admin/imports/b1/commit')[0].body).toEqual({ retailPricesPublic: true })
  })

  it('summarises counts', () => {
    expect(countsSummary(batch)).toBe('1 waiting · 1 accept')
  })
})

describe('phones', () => {
  it('shows a pairing QR code and the readable code', async () => {
    renderApp(
      '/admin/devices',
      api({
        '/admin/devices': { items: [] },
        '/admin/devices/pairing-codes': new Reply(201, {
          code: 'ABCD2345',
          url: 'https://shop.example/pair?code=ABCD2345',
          expiresAt: new Date(Date.now() + 600_000).toISOString(),
        }),
      }),
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Show a pairing code' }))
    expect(await screen.findByRole('img', { name: /Pairing QR code/ })).toBeInTheDocument()
    expect(screen.getByText('ABCD-2345')).toBeInTheDocument()
    expect(screen.getByText(/expires in (9:5\d|10:00)/)).toBeInTheDocument()
  })

  it('revokes a phone after confirming', async () => {
    const { calls } = renderApp(
      '/admin/devices',
      api({
        '/admin/devices': { items: [{ id: 'd1', name: 'Shop phone', createdAt: '2026-10-01T10:00:00Z' }] },
        '/admin/devices/d1': new Reply(204),
      }),
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Revoke Shop phone' }))
    expect(find(calls, 'DELETE', '/admin/devices/d1')).toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: 'Revoke' }))
    await waitFor(() => expect(find(calls, 'DELETE', '/admin/devices/d1')).toHaveLength(1))
  })

  it('is hidden from the menu on a paired phone', async () => {
    renderApp('/admin', api({ '/auth/session': { ...session, kind: 'device' }, '/admin/products': { items: [] } }))
    await screen.findByRole('button', { name: 'Unpair' })
    expect(screen.queryByRole('link', { name: 'Phones' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Owner' })).not.toBeInTheDocument()
  })

  it('draws a QR code as one SVG path', () => {
    const { container } = render(<QRCode value="https://shop.example/pair?code=ABCD2345" label="code" />)
    const svg = container.querySelector('svg')!
    const size = Number(svg.getAttribute('viewBox')!.split(' ')[2])
    expect(size).toBeGreaterThanOrEqual(25) // 21 modules (version 1) plus a 2-module border each side
    expect(container.querySelector('path')!.getAttribute('d')).toMatch(/^M\d+ \d+h1v1h-1z/)
  })

  it('formats codes for reading out', () => {
    expect(formatCode('ABCD2345')).toBe('ABCD-2345')
  })
})

describe('account', () => {
  it('checks the repeat before asking the server, then changes the password', async () => {
    const { calls } = renderApp('/admin/account', api({ '/auth/password': new Reply(204) }))
    fireEvent.change(await screen.findByLabelText('Current password'), { target: { value: 'old password here' } })
    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'correct horse battery' } })
    fireEvent.change(screen.getByLabelText('Repeat the new password'), { target: { value: 'correct horse batter' } })
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
    expect(screen.getByRole('alert')).toHaveTextContent('not the same')
    expect(find(calls, 'PUT', '/auth/password')).toHaveLength(0)

    fireEvent.change(screen.getByLabelText('Repeat the new password'), { target: { value: 'correct horse battery' } })
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
    expect(await screen.findByText(/Password changed/)).toBeInTheDocument()
    expect(find(calls, 'PUT', '/auth/password')[0].body).toEqual({
      currentPassword: 'old password here',
      newPassword: 'correct horse battery',
    })
  })

  it("shows the server's verdict on the current password", async () => {
    renderApp(
      '/admin/account',
      api({ '/auth/password': problem(422, 'Validation failed', [{ field: 'currentPassword', message: 'is incorrect' }]) }),
    )
    fireEvent.change(await screen.findByLabelText('Current password'), { target: { value: 'wrong password!' } })
    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'correct horse battery' } })
    fireEvent.change(screen.getByLabelText('Repeat the new password'), { target: { value: 'correct horse battery' } })
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
    expect(await screen.findByText('Current password is incorrect')).toBeInTheDocument()
  })
})

describe('subscriber export', () => {
  afterEach(() => vi.restoreAllMocks())

  it('downloads the CSV with the name the server gives it', async () => {
    const saved: string[] = []
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:csv')
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      saved.push(this.download)
    })
    renderApp(
      '/admin',
      api({
        '/admin/products': { items: [] },
        '/admin/subscribers/stats': { confirmed: 1 },
        '/admin/subscribers/export': new Reply(200, 'email,name\nana@example.com,Ana\n', {
          'Content-Type': 'text/csv',
          'Content-Disposition': 'attachment; filename="subscribers-2026-10-08.csv"',
        }),
      }),
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Download confirmed subscribers (CSV)' }))
    await waitFor(() => expect(saved).toEqual(['subscribers-2026-10-08.csv']))
  })
})

describe('activity log helpers', () => {
  it('names actions and links to what they changed', () => {
    expect(describeAction('product.update')).toBe('Edited product')
    expect(describeAction('device.pair')).toBe('Paired a phone')
    expect(describeAction('something.new')).toBe('something.new')
    expect(entityLink('category', 'c1')).toBe('/admin/categories/c1')
    expect(entityLink('brand', 'b1')).toBeUndefined()
  })

  it('lists the fields that changed', () => {
    expect(changedFields({ name: 'Grips', tags: ['a'], same: 1 }, { name: 'Star Grips', tags: ['a', 'b'], same: 1 })).toEqual([
      { field: 'name', before: 'Grips', after: 'Star Grips' },
      { field: 'tags', before: '["a"]', after: '["a","b"]' },
    ])
    expect(changedFields(null, { name: 'New' })).toEqual([{ field: 'name', before: '—', after: 'New' }])
    expect(changedFields({ name: 'Old' }, null)).toEqual([{ field: 'name', before: 'Old', after: '—' }])
    expect(changedFields(undefined, undefined)).toEqual([])
    expect(changedFields({ note: 'x'.repeat(200) }, {})[0].before).toHaveLength(118)
  })
})

describe('attribute display', () => {
  const field = (over: Partial<FormField>): FormField => ({
    key: 'k',
    label: 'K',
    inputType: 'select',
    dataType: 'enum',
    required: false,
    isVariantAxis: false,
    ...over,
  })

  it('uses option labels, ranges and units', () => {
    const valve = field({ options: [{ value: 'av', label: 'A/V (Schrader)' }] })
    expect(formatAttributeValue(valve, 'av')).toBe('A/V (Schrader)')
    expect(formatAttributeValue(field({ dataType: 'number_range', unit: 'in' }), { low: 1.95, high: 2.125 })).toBe('1.95–2.125 in')
    expect(formatAttributeValue(field({ dataType: 'number_range' }), { low: 1.95, high: 1.95 })).toBe('1.95')
    expect(formatAttributeValue(field({ dataType: 'number', unit: 'mm' }), 48)).toBe('48 mm')
    expect(formatAttributeValue(field({ dataType: 'boolean' }), true)).toBe('Yes')
    expect(formatAttributeValue(undefined, 'raw')).toBe('raw')
  })
})
