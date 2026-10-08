import type { FormField } from '../api/adminTypes'

const trim = (n: number) => String(Number(n.toFixed(4)))

/**
 * An attribute value in its API shape, written for people using the
 * category's form schema: option labels instead of value tokens, ranges as
 * "1.95–2.125", units appended. Without a field it falls back to plain text.
 */
export function formatAttributeValue(field: FormField | undefined, value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  const unit = field?.unit ? ` ${field.unit}` : ''
  const label = (v: unknown) => field?.options?.find((o) => o.value === v)?.label ?? String(v)
  if (Array.isArray(value)) return value.map(label).join(', ')
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (typeof value === 'number') return trim(value) + unit
  if (typeof value === 'object' && 'low' in value && 'high' in value) {
    const { low, high } = value as { low: number; high: number }
    return (low === high ? trim(low) : `${trim(low)}–${trim(high)}`) + unit
  }
  if (typeof value === 'object') return JSON.stringify(value)
  return label(value) + (field?.dataType === 'number' ? unit : '')
}
