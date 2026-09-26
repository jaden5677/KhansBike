import { useParams } from 'react-router'
import { useFitment } from '../api/catalog'
import { Loadable } from '../components/Loadable'
import { SearchGroups } from '../components/SearchGroups'

/** Everything that fits one wheel size, across all categories. */
export function FitmentPage() {
  const { wheelSize = '' } = useParams()
  const results = useFitment(wheelSize)
  return (
    <>
      <title>{`Fits ${wheelSize} | Khan's Bike Zone`}</title>
      <h1>Parts that fit {wheelSize}</h1>
      <Loadable query={results}>
        {(groups) =>
          groups.length === 0 ? (
            <p>We have nothing listed for this wheel size yet.</p>
          ) : (
            <SearchGroups
              groups={groups}
              seeAll={(g) => `/c/${g.category.slug}?attr.wheel_size=${encodeURIComponent(wheelSize)}`}
            />
          )
        }
      </Loadable>
    </>
  )
}
