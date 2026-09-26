import { useState } from 'react'
import type { AttrValue, FormField } from '../api/adminTypes'

interface Props {
  field: FormField
  value: AttrValue | undefined
  onChange: (value: AttrValue) => void
  error?: string
}

/**
 * One attribute input, chosen by the attribute's data type from the
 * category's form schema. It speaks the API's value shapes directly
 * (see api/adminTypes AttrValue), so nothing needs converting on save.
 */
export function AttributeField({ field, value, onChange, error }: Props) {
  const label = (
    <span>
      {field.label}
      {field.unit && <span className="muted"> ({field.unit})</span>}
      {field.required && <span className="muted"> · needed to publish</span>}
    </span>
  )
  return (
    <div className="field">
      {field.dataType === 'multi_enum' ? (
        <fieldset>
          <legend>{label}</legend>
          <MultiChoice field={field} value={value} onChange={onChange} />
        </fieldset>
      ) : field.dataType === 'number_range' ? (
        <fieldset>
          <legend>{label}</legend>
          <RangeInput field={field} value={value} onChange={onChange} />
        </fieldset>
      ) : (
        <label>
          {label}
          <SingleInput field={field} value={value} onChange={onChange} invalid={Boolean(error)} />
        </label>
      )}
      {field.helpText && <span className="muted">{field.helpText}</span>}
      {error && <span className="error-text">{error}</span>}
    </div>
  )
}

function SingleInput({ field, value, onChange, invalid }: Omit<Props, 'error'> & { invalid: boolean }) {
  const common = { 'aria-invalid': invalid || undefined }
  switch (field.dataType) {
    case 'number':
      return (
        <input
          type="number"
          step="any"
          inputMode="decimal"
          value={typeof value === 'number' ? value : ''}
          onChange={(e) => onChange(e.target.value === '' ? null : Number(e.target.value))}
          {...common}
        />
      )
    case 'boolean':
      // Three states: yes, no, or not stated.
      return (
        <select
          value={value === true ? 'true' : value === false ? 'false' : ''}
          onChange={(e) => onChange(e.target.value === '' ? null : e.target.value === 'true')}
          {...common}
        >
          <option value="">—</option>
          <option value="true">Yes</option>
          <option value="false">No</option>
        </select>
      )
    case 'enum':
    case 'color':
      return (
        <select value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value || null)} {...common}>
          <option value="">—</option>
          {field.options?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )
    default: // text
      return (
        <input value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value)} {...common} />
      )
  }
}

function MultiChoice({ field, value, onChange }: Omit<Props, 'error'>) {
  const selected = Array.isArray(value) ? value : []
  return (
    <div className="checkbox-list">
      {field.options?.map((o) => (
        <label key={o.value}>
          <input
            type="checkbox"
            checked={selected.includes(o.value)}
            onChange={(e) =>
              onChange(e.target.checked ? [...selected, o.value] : selected.filter((v) => v !== o.value))
            }
          />
          {o.label}
        </label>
      ))}
    </div>
  )
}

/**
 * A low–high pair, e.g. a tyre width of 1.95–2.125. The API only accepts
 * both ends, so half-typed input stays local until both are numbers.
 */
function RangeInput({ field, value, onChange }: Omit<Props, 'error'>) {
  const initial = value && typeof value === 'object' && 'low' in value ? value : undefined
  const [low, setLow] = useState(initial ? String(initial.low) : '')
  const [high, setHigh] = useState(initial ? String(initial.high) : '')

  function update(nextLow: string, nextHigh: string) {
    setLow(nextLow)
    setHigh(nextHigh)
    const l = Number(nextLow)
    const h = Number(nextHigh)
    const complete = nextLow.trim() !== '' && nextHigh.trim() !== '' && Number.isFinite(l) && Number.isFinite(h)
    onChange(complete ? { low: l, high: h } : null)
  }

  return (
    <div className="range-inputs">
      <input
        type="number"
        step="any"
        inputMode="decimal"
        aria-label={`${field.label} from`}
        value={low}
        onChange={(e) => update(e.target.value, high)}
      />
      <span aria-hidden="true">–</span>
      <input
        type="number"
        step="any"
        inputMode="decimal"
        aria-label={`${field.label} to`}
        value={high}
        onChange={(e) => update(low, e.target.value)}
      />
    </div>
  )
}
