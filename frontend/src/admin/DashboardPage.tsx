import { Link } from 'react-router'
import { useAdminProducts, useProductCount, useSubscriberStats } from '../api/admin'
import { statusLabels, type ProductStatus } from '../api/adminTypes'
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
      </section>
      <section>
        <h2>Recently edited</h2>
        <Loadable query={products}>{(data) => <ProductTable products={data.pages[0]?.items ?? []} />}</Loadable>
      </section>
    </>
  )
}
