import { describe, expect, it } from 'vitest'
import {
  activeFilterCount,
  clearFilters,
  listingQuery,
  rangeValue,
  selectedValues,
  setRange,
  toggleValue,
  withParam,
} from './filters'

const p = (s: string) => new URLSearchParams(s)

describe('selectedValues', () => {
  it('reads comma-separated and repeated values', () => {
    expect(selectedValues(p('attr.colour=red,blue&attr.colour=green'), 'colour')).toEqual(['red', 'blue', 'green'])
    expect(selectedValues(p(''), 'colour')).toEqual([])
  })
})

describe('toggleValue', () => {
  it('adds, then removes, a value', () => {
    const on = toggleValue(p('attr.colour=red'), 'colour', 'blue')
    expect(on.get('attr.colour')).toBe('red,blue')
    const off = toggleValue(on, 'colour', 'red')
    expect(off.get('attr.colour')).toBe('blue')
    expect(toggleValue(off, 'colour', 'blue').has('attr.colour')).toBe(false)
  })

  it('drops the cursor and leaves the input untouched', () => {
    const input = p('cursor=abc&sort=name')
    const next = toggleValue(input, 'colour', 'red')
    expect(next.has('cursor')).toBe(false)
    expect(next.get('sort')).toBe('name')
    expect(input.has('attr.colour')).toBe(false)
  })
})

describe('ranges', () => {
  it('sets, reads and clears bounds', () => {
    const next = setRange(p(''), 'wheel_size', { min: 20, max: 26 })
    expect(next.toString()).toBe('attr.wheel_size_min=20&attr.wheel_size_max=26')
    expect(rangeValue(next, 'wheel_size')).toEqual({ min: 20, max: 26 })
    expect(setRange(next, 'wheel_size', { max: 26 }).has('attr.wheel_size_min')).toBe(false)
  })

  it('ignores values that are not numbers', () => {
    expect(rangeValue(p('attr.size_min=abc'), 'size')).toEqual({ min: undefined, max: undefined })
  })
})

describe('clearFilters and activeFilterCount', () => {
  it('keeps the search text and sort order', () => {
    const params = p('q=tyre&sort=name&attr.colour=red&attr.size_min=1&attr.size_max=2')
    expect(activeFilterCount(params)).toBe(2)
    expect(clearFilters(params).toString()).toBe('q=tyre&sort=name')
  })
})

describe('withParam', () => {
  it('sets and removes a parameter', () => {
    expect(withParam(p('sort=name'), 'sort', '-created').get('sort')).toBe('-created')
    expect(withParam(p('sort=name'), 'sort', undefined).has('sort')).toBe(false)
  })
})

describe('listingQuery', () => {
  it('forwards only listing parameters, sorted, plus extras', () => {
    const q = listingQuery(p('utm_source=x&sort=name&attr.colour=red&cursor=abc&q='), { category: 'tyres' })
    expect(q.toString()).toBe('attr.colour=red&category=tyres&sort=name')
  })
})
