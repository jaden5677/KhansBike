import type { Money } from '../api/types'

const formatters = new Map<string, Intl.NumberFormat>()

/** "$1,250.00" for TTD, "US$12.50" for USD. */
export function formatMoney(money: Money): string {
  let fmt = formatters.get(money.currency)
  if (!fmt) {
    fmt = new Intl.NumberFormat('en-TT', { style: 'currency', currency: money.currency })
    formatters.set(money.currency, fmt)
  }
  // The amount is a decimal string; Number() is exact enough for display
  // (we never do arithmetic on it here).
  return fmt.format(Number(money.amount))
}

/**
 * A price, or "Ask in store" when the API sends null. The server decides
 * which prices are public; this component never has to.
 */
export function Price({ money, from = false }: { money: Money | null; from?: boolean }) {
  if (!money) return <span className="price price-ask">Ask in store</span>
  return (
    <span className="price">
      {from && <span className="muted">From </span>}
      {formatMoney(money)}
    </span>
  )
}
