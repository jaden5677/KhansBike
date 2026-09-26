import { Link } from 'react-router'
import type { SearchGroup } from '../api/types'
import { ProductGrid } from './ProductGrid'

/**
 * Results grouped by category, used by search and fitment. Each group shows
 * a few products and links to the full, filterable category listing.
 */
export function SearchGroups({ groups, seeAll }: { groups: SearchGroup[]; seeAll: (group: SearchGroup) => string }) {
  return (
    <>
      {groups.map((group) => (
        <section key={group.category.id} className="group">
          <h2>
            {group.category.name} <span className="muted">({group.total})</span>
          </h2>
          <ProductGrid products={group.products} />
          {group.total > group.products.length && (
            <p>
              <Link to={seeAll(group)}>
                See all {group.total} in {group.category.name}
              </Link>
            </p>
          )}
        </section>
      ))}
    </>
  )
}
