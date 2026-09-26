import type { StockStatus } from '../api/types'

const labels: Record<StockStatus, string | null> = {
  in_stock: 'In stock',
  low: 'Low stock',
  out: 'Out of stock',
  special_order: 'Special order',
  unknown: null, // say nothing rather than guess
}

export function StockBadge({ status }: { status: StockStatus }) {
  const label = labels[status]
  if (!label) return null
  return <span className={`badge badge-${status}`}>{label}</span>
}
