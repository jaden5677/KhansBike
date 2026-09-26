import { useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useAdminProducts } from '../api/admin'
import { statusLabels, type AdminProductSummary, type ProductStatus } from '../api/adminTypes'
import { Loadable } from '../components/Loadable'
import { Price } from '../components/Price'
import { withParam } from '../lib/filters'
import styles from './admin.module.css'

const dateFormat = new Intl.DateTimeFormat('en-TT', { dateStyle: 'medium' })

export function ProductTable({ products }: { products: AdminProductSummary[] }) {
  if (products.length === 0) return <p className="muted">No products.</p>
  return (
    <table className={styles.table}>
      <thead>
        <tr>
          <th>Name</th>
          <th>Category</th>
          <th>Status</th>
          <th>Public price</th>
          <th>Edited</th>
        </tr>
      </thead>
      <tbody>
        {products.map((p) => (
          <tr key={p.id}>
            <td>
              <Link to={`/admin/products/${p.id}`}>{p.name}</Link>
              {p.variantCount > 1 && <span className="muted"> · {p.variantCount} variants</span>}
            </td>
            <td>{p.category.name}</td>
            <td>
              <span className={`badge ${styles[`status_${p.status}`] ?? ''}`}>{statusLabels[p.status]}</span>
            </td>
            <td>
              {/* The list carries only the public price; private ones are in the editor. */}
              {p.fromPrice ? <Price money={p.fromPrice} /> : <span className="muted">Not shown</span>}
            </td>
            <td>{dateFormat.format(new Date(p.updatedAt))}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

/** Every product, any status, newest edit first; search and status filter live in the URL. */
export function ProductsPage() {
  const [params, setParams] = useSearchParams()
  const [text, setText] = useState(params.get('q') ?? '')
  const query = new URLSearchParams()
  for (const name of ['q', 'status']) {
    const value = params.get(name)
    if (value) query.set(name, value)
  }
  const list = useAdminProducts(query)

  function search(e: FormEvent) {
    e.preventDefault()
    setParams(withParam(params, 'q', text.trim() || undefined))
  }

  return (
    <>
      <title>Products | Khan's Bike Zone admin</title>
      <div className={styles.titleRow}>
        <h1>Products</h1>
        <Link to="/admin/products/new" className="button">
          Add product
        </Link>
      </div>
      <div className={styles.toolbar}>
        <form role="search" onSubmit={search} className={styles.inline}>
          <input
            type="search"
            aria-label="Search products"
            placeholder="Name, SKU or supplier code"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <button type="submit">Search</button>
        </form>
        <label>
          Status{' '}
          <select
            value={params.get('status') ?? ''}
            onChange={(e) => setParams(withParam(params, 'status', e.target.value || undefined))}
          >
            <option value="">All</option>
            {(Object.keys(statusLabels) as ProductStatus[]).map((s) => (
              <option key={s} value={s}>
                {statusLabels[s]}
              </option>
            ))}
          </select>
        </label>
      </div>
      <Loadable query={list}>
        {(data) => (
          <>
            <p className="muted">{data.pages[0]?.total ?? 0} products</p>
            <ProductTable products={data.pages.flatMap((p) => p.items)} />
            {list.hasNextPage && (
              <p className="load-more">
                <button type="button" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
                  {list.isFetchingNextPage ? 'Loading…' : 'Load more'}
                </button>
              </p>
            )}
          </>
        )}
      </Loadable>
    </>
  )
}
