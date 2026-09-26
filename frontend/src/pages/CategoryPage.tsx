import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { useCategory, useFacets } from '../api/catalog'
import { Breadcrumbs } from '../components/Breadcrumbs'
import { FilterPanel } from '../components/FilterPanel'
import { Loadable } from '../components/Loadable'
import { ProductList } from '../components/ProductList'
import { activeFilterCount, clearFilters, listingQuery, withParam } from '../lib/filters'
import styles from './CategoryPage.module.css'

const sorts = [
  { value: 'featured', label: 'Featured' },
  { value: 'name', label: 'Name A–Z' },
  { value: '-created', label: 'Newest' },
]

/**
 * A category listing with filters. Everything the visitor chooses (filters,
 * sort, search text) is read from and written to the URL; the page just
 * forwards it to the API.
 */
export function CategoryPage() {
  const { slug = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const [filtersOpen, setFiltersOpen] = useState(false)
  const category = useCategory(slug)
  const facets = useFacets(slug, listingQuery(params))

  const q = params.get('q') ?? ''
  const filterCount = activeFilterCount(params)
  // preventScrollReset: changing a filter should not jump to the top.
  const update = (next: URLSearchParams) => setParams(next, { preventScrollReset: true })

  return (
    <Loadable query={category}>
      {(c) => (
        <>
          <title>{`${c.name} | Khan's Bike Zone`}</title>
          <Breadcrumbs trail={c.breadcrumbs} current={c.name} />
          <h1>{c.name}</h1>
          {c.description && <p className={styles.description}>{c.description}</p>}
          {c.children.length > 0 && (
            <nav aria-label="Subcategories">
              <ul className="chips">
                {c.children.map((child) => (
                  <li key={child.id}>
                    <Link to={`/c/${child.slug}`}>{child.name}</Link>
                  </li>
                ))}
              </ul>
            </nav>
          )}

          <div className={styles.layout}>
            <aside className={`${styles.filters} ${filtersOpen ? styles.open : ''}`} aria-label="Filters">
              <div className={styles.filtersHeader}>
                <h2>Filters</h2>
                {filterCount > 0 && (
                  <button type="button" className="link-button" onClick={() => update(clearFilters(params))}>
                    Clear all
                  </button>
                )}
              </div>
              {facets.data ? (
                <FilterPanel facets={facets.data} params={params} onChange={update} />
              ) : (
                <p className="muted">{facets.isError ? 'Filters are unavailable right now.' : 'Loading…'}</p>
              )}
            </aside>

            <div className={styles.results}>
              <div className={styles.toolbar}>
                <button
                  type="button"
                  className={styles.filtersToggle}
                  aria-expanded={filtersOpen}
                  onClick={() => setFiltersOpen((o) => !o)}
                >
                  Filters{filterCount > 0 ? ` (${filterCount})` : ''}
                </button>
                <label>
                  Sort by{' '}
                  <select
                    // The API's default order: best match when searching, else featured.
                    value={params.get('sort') ?? (q ? 'relevance' : 'featured')}
                    onChange={(e) => update(withParam(params, 'sort', e.target.value))}
                  >
                    {q && <option value="relevance">Best match</option>}
                    {sorts.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              {q && (
                <p>
                  Matching “{q}”{' '}
                  <button
                    type="button"
                    className="link-button"
                    // Relevance means nothing without search text.
                    onClick={() => update(withParam(withParam(params, 'q', undefined), 'sort', undefined))}
                  >
                    Show all
                  </button>
                </p>
              )}
              <ProductList query={listingQuery(params, { category: slug })} />
            </div>
          </div>
        </>
      )}
    </Loadable>
  )
}
