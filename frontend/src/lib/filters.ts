// Listing filters live in the page URL, in the same shape the API accepts:
//
//   ?q=helmet&sort=name&brand=shimano
//   &attr.colour=red,blue          any of these values
//   &attr.wheel_size_min=20&attr.wheel_size_max=26   a numeric range
//
// Keeping them in the URL makes a filtered view shareable and lets the back
// button undo a filter, and it means React holds no copy that could drift
// from what the address bar says. These helpers are the only code that edits
// those parameters. Each returns a new URLSearchParams and never mutates its
// input, and each drops the pagination cursor, because a cursor from one
// result set means nothing in another.

const ATTR = 'attr.'

/** Parameters a listing forwards to the API; anything else (utm_*, …) is ignored. */
function isListingParam(name: string): boolean {
  return name === 'q' || name === 'sort' || name === 'brand' || name.startsWith(ATTR)
}

function copy(params: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(params)
  next.delete('cursor')
  return next
}

/** The values chosen for an attribute: comma-separated and/or repeated. */
export function selectedValues(params: URLSearchParams, key: string): string[] {
  return params
    .getAll(ATTR + key)
    .flatMap((v) => v.split(','))
    .map((v) => v.trim())
    .filter(Boolean)
}

/** Ticks or unticks one value of an attribute filter. */
export function toggleValue(params: URLSearchParams, key: string, value: string): URLSearchParams {
  const current = selectedValues(params, key)
  const values = current.includes(value) ? current.filter((v) => v !== value) : [...current, value]
  const next = copy(params)
  next.delete(ATTR + key)
  if (values.length > 0) next.set(ATTR + key, values.join(','))
  return next
}

export interface Range {
  min?: number
  max?: number
}

function readNumber(params: URLSearchParams, name: string): number | undefined {
  const raw = params.get(name)
  if (raw === null || raw.trim() === '') return undefined
  const n = Number(raw)
  return Number.isFinite(n) ? n : undefined
}

export function rangeValue(params: URLSearchParams, key: string): Range {
  return { min: readNumber(params, `${ATTR}${key}_min`), max: readNumber(params, `${ATTR}${key}_max`) }
}

/** Sets (or, with undefined bounds, clears) a numeric range filter. */
export function setRange(params: URLSearchParams, key: string, range: Range): URLSearchParams {
  const next = copy(params)
  for (const [bound, value] of [
    ['min', range.min],
    ['max', range.max],
  ] as const) {
    const name = `${ATTR}${key}_${bound}`
    if (value === undefined) next.delete(name)
    else next.set(name, String(value))
  }
  return next
}

/** Sets or removes a plain parameter such as sort or q. */
export function withParam(params: URLSearchParams, name: string, value: string | undefined): URLSearchParams {
  const next = copy(params)
  if (value) next.set(name, value)
  else next.delete(name)
  return next
}

/** Removes every attribute filter, keeping the search text and sort order. */
export function clearFilters(params: URLSearchParams): URLSearchParams {
  const next = copy(params)
  for (const name of [...next.keys()]) {
    if (name.startsWith(ATTR)) next.delete(name)
  }
  return next
}

/** How many attribute filters are active (a range counts once). */
export function activeFilterCount(params: URLSearchParams): number {
  const keys = new Set<string>()
  for (const name of params.keys()) {
    if (name.startsWith(ATTR)) keys.add(name.slice(ATTR.length).replace(/_(min|max)$/, ''))
  }
  return keys.size
}

/**
 * The parameters to send to the API for this page, sorted so the same filters
 * in a different order share one cache entry.
 */
export function listingQuery(params: URLSearchParams, extra: Record<string, string> = {}): URLSearchParams {
  const out = new URLSearchParams()
  for (const [name, value] of params) {
    if (isListingParam(name) && value !== '') out.append(name, value)
  }
  for (const [name, value] of Object.entries(extra)) out.set(name, value)
  out.sort()
  return out
}
