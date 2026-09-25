import { useProductList } from '../api/catalog'
import { Loadable } from './Loadable'
import { ProductGrid } from './ProductGrid'

interface Props {
  /** API parameters (see lib/filters listingQuery). */
  query: URLSearchParams
  limit?: number
  /** Offer "Load more" when there are further pages. */
  paginate?: boolean
  empty?: string
}

/** A product grid for any listing query, growing with "Load more". */
export function ProductList({ query, limit, paginate = true, empty = 'No products match.' }: Props) {
  const list = useProductList(query, limit)
  return (
    <Loadable query={list}>
      {(data) => {
        const products = data.pages.flatMap((page) => page.items)
        const total = data.pages[0]?.total
        if (products.length === 0) return <p className="muted">{empty}</p>
        return (
          <>
            {paginate && total !== undefined && (
              <p className="muted" aria-live="polite">
                {total} {total === 1 ? 'product' : 'products'}
              </p>
            )}
            <ProductGrid products={products} />
            {paginate && list.hasNextPage && (
              <p className="load-more">
                <button type="button" onClick={() => list.fetchNextPage()} disabled={list.isFetchingNextPage}>
                  {list.isFetchingNextPage ? 'Loading…' : 'Load more'}
                </button>
              </p>
            )}
          </>
        )
      }}
    </Loadable>
  )
}
