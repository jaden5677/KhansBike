import { useState, type FormEvent } from 'react'
import type { Facet } from '../api/types'
import { rangeValue, selectedValues, setRange, toggleValue, type Range } from '../lib/filters'
import styles from './FilterPanel.module.css'

interface Props {
  facets: Facet[]
  params: URLSearchParams
  onChange: (next: URLSearchParams) => void
}

/**
 * The filter sidebar. It is fully controlled by the URL: it reads the
 * current choices from `params` and reports every change as new params, so
 * it holds no filter state of its own (only half-typed range inputs).
 */
export function FilterPanel({ facets, params, onChange }: Props) {
  if (facets.length === 0) return <p className="muted">No filters for this category.</p>
  return (
    <div className={styles.panel}>
      {facets.map((facet) =>
        facet.values && facet.values.length > 0 ? (
          <fieldset key={facet.key} className={styles.facet}>
            <legend>{facet.label}</legend>
            {facet.values.map((v) => {
              const checked = selectedValues(params, facet.key).includes(v.value)
              return (
                <label key={v.value} className={styles.option}>
                  <input
                    type="checkbox"
                    checked={checked}
                    // An option with no matches can still be unticked.
                    disabled={v.count === 0 && !checked}
                    onChange={() => onChange(toggleValue(params, facet.key, v.value))}
                  />
                  {v.label} <span className="muted">({v.count})</span>
                </label>
              )
            })}
          </fieldset>
        ) : facet.numMin !== undefined && facet.numMax !== undefined ? (
          <RangeFilter
            // A new key (after the URL changes) resets the half-typed inputs.
            key={`${facet.key}:${params.toString()}`}
            facet={facet}
            value={rangeValue(params, facet.key)}
            onApply={(range) => onChange(setRange(params, facet.key, range))}
          />
        ) : null,
      )}
    </div>
  )
}

function RangeFilter({ facet, value, onApply }: { facet: Facet; value: Range; onApply: (r: Range) => void }) {
  const [min, setMin] = useState(value.min?.toString() ?? '')
  const [max, setMax] = useState(value.max?.toString() ?? '')
  const toNumber = (s: string) => (s.trim() === '' ? undefined : Number(s))

  function apply(e: FormEvent) {
    e.preventDefault()
    onApply({ min: toNumber(min), max: toNumber(max) })
  }

  return (
    <form className={styles.facet} onSubmit={apply} aria-label={facet.label}>
      <fieldset>
        <legend>{facet.label}</legend>
        <div className={styles.range}>
          <input
            type="number"
            inputMode="decimal"
            aria-label={`${facet.label} from`}
            placeholder={String(facet.numMin)}
            value={min}
            onChange={(e) => setMin(e.target.value)}
          />
          <span aria-hidden="true">–</span>
          <input
            type="number"
            inputMode="decimal"
            aria-label={`${facet.label} to`}
            placeholder={String(facet.numMax)}
            value={max}
            onChange={(e) => setMax(e.target.value)}
          />
          <button type="submit">Apply</button>
        </div>
      </fieldset>
    </form>
  )
}
