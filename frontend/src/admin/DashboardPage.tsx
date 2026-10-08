import { useState } from 'react'
import { Link } from 'react-router'
import { useAdminProducts, useProductCount, useSubscriberStats } from '../api/admin'
import { statusLabels, type ProductStatus } from '../api/adminTypes'
import { download } from '../api/client'
import { ErrorText } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'
import { ProductTable } from './ProductsPage'

const recent = new URLSearchParams()

function StatusCount({ status }: { status: ProductStatus }) {
  const count = useProductCount(status)
  return (
    <Link to={`/admin/products?status=${status}`} className={styles.stat}>
      <strong>{count.data ?? '…'}</strong>
      <span>{statusLabels[status]}</span>
    </Link>
  )
}

/** Saves the confirmed subscribers as a CSV file (for a mail-merge or newsletter tool). */
function ExportButton() {
  const [state, setState] = useState<{ busy: boolean; error: unknown }>({ busy: false, error: null })
  async function exportCSV() {
    setState({ busy: true, error: null })
    try {
      await download('/admin/subscribers/export', 'subscribers.csv')
      setState({ busy: false, error: null })
    } catch (error) {
      setState({ busy: false, error })
    }
  }
  return (
    <>
      <button type="button" onClick={exportCSV} disabled={state.busy}>
        {state.busy ? 'Preparing…' : 'Download confirmed subscribers (CSV)'}
      </button>
      <ErrorText error={state.error} />
    </>
  )
}

export function DashboardPage() {
  const subscribers = useSubscriberStats()
  const products = useAdminProducts(recent, 8)
  return (
    <>
      <title>Dashboard | Khan's Bike Zone admin</title>
      <h1>Dashboard</h1>
      <section>
        <h2>Products</h2>
        <div className={styles.stats}>
          <StatusCount status="active" />
          <StatusCount status="draft" />
          {/* Imports put products with warnings here until someone checks them. */}
          <StatusCount status="needs_review" />
          <StatusCount status="discontinued" />
        </div>
        <p>
          <Link to="/admin/products/new">Add a product</Link>
        </p>
      </section>
      <section>
        <h2>Mailing list</h2>
        <Loadable query={subscribers}>
          {(s) => (
            <p>
              {s.confirmed ?? 0} confirmed · {s.pending ?? 0} waiting to confirm · {s.unsubscribed ?? 0} unsubscribed
            </p>
          )}
        </Loadable>
        <ExportButton />
      </section>
      <section>
        <h2>Recently edited</h2>
        <Loadable query={products}>{(data) => <ProductTable products={data.pages[0]?.items ?? []} />}</Loadable>
      </section>
    </>
  )
}
