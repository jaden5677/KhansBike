import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { priceTiers, type ImportBatch, type ImportDecision, type ImportRow } from '../api/adminTypes'
import { useFormSchema } from '../api/admin'
import { useDecideRow, useFinishBatch, useImportBatch, useImportRows, useSkipAllPending } from '../api/adminImports'
import { ErrorText } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import { formatAttributeValue } from '../lib/attributes'
import { withParam } from '../lib/filters'
import styles from './admin.module.css'
import { batchStatusLabels, countsSummary } from './ImportsPage'

const decisionLabels: Record<ImportDecision, string> = {
  pending: 'Waiting',
  accept: 'Accept',
  merge: 'Merge',
  skip: 'Skip',
}

const isOpen = (b: ImportBatch) => b.status === 'dry_run' || b.status === 'reviewing'

/** Reviewing one uploaded workbook, row by row, then applying or discarding it. */
export function ImportBatchPage() {
  const { id = '' } = useParams()
  const batch = useImportBatch(id)
  return (
    <>
      <p>
        <Link to="/admin/imports">← Imports</Link>
      </p>
      <Loadable query={batch}>{(b) => <BatchReview batch={b} />}</Loadable>
    </>
  )
}

function BatchReview({ batch }: { batch: ImportBatch }) {
  const [params, setParams] = useSearchParams()
  const decision = (params.get('decision') || undefined) as ImportDecision | undefined
  const issuesOnly = params.get('issues') === 'true'
  const rows = useImportRows(batch.id, { decision, issuesOnly })
  const open = isOpen(batch)
  const pending = batch.counts?.pending ?? 0

  return (
    <>
      <title>{`${batch.filename} | Imports | Khan's Bike Zone admin`}</title>
      <h1>{batch.filename}</h1>
      <p>
        <span className="badge">{batchStatusLabels[batch.status]}</span> {countsSummary(batch)}
      </p>

      {open ? (
        <FinishPanel batch={batch} pending={pending} />
      ) : (
        <p className="notice">
          {batch.status === 'committed'
            ? 'This workbook has been applied to the catalogue. Products that had warnings are waiting in '
            : 'This import was discarded; nothing in the catalogue changed.'}
          {batch.status === 'committed' && <Link to="/admin/products?status=needs_review">Needs review</Link>}
        </p>
      )}

      <div className={styles.toolbar}>
        <label>
          Show{' '}
          <select
            value={decision ?? ''}
            onChange={(e) => setParams(withParam(params, 'decision', e.target.value || undefined), { replace: true })}
          >
            <option value="">All rows</option>
            {(Object.keys(decisionLabels) as ImportDecision[]).map((d) => (
              <option key={d} value={d}>
                {decisionLabels[d]}
              </option>
            ))}
          </select>
        </label>
        <label className={styles.inline}>
          <input
            type="checkbox"
            checked={issuesOnly}
            onChange={(e) => setParams(withParam(params, 'issues', e.target.checked ? 'true' : undefined), { replace: true })}
          />
          Only rows with issues
        </label>
      </div>

      <Loadable query={rows}>
        {(data) => {
          const list = data.pages.flatMap((p) => p.items)
          if (list.length === 0) return <p className="muted">No rows match.</p>
          return (
            <>
              <ul className={styles.importRows}>
                {list.map((row) => (
                  <ImportRowCard key={row.id} batchId={batch.id} row={row} editable={open} />
                ))}
              </ul>
              {rows.hasNextPage && (
                <p className="load-more">
                  <button type="button" onClick={() => rows.fetchNextPage()} disabled={rows.isFetchingNextPage}>
                    {rows.isFetchingNextPage ? 'Loading…' : 'Load more rows'}
                  </button>
                </p>
              )}
            </>
          )
        }}
      </Loadable>
    </>
  )
}

/** Skip-the-rest, apply and discard. Applying is blocked while rows wait. */
function FinishPanel({ batch, pending }: { batch: ImportBatch; pending: number }) {
  const finish = useFinishBatch(batch.id)
  const [publicPrices, setPublicPrices] = useState(false)
  const [confirmAbort, setConfirmAbort] = useState(false)
  const skipAll = useSkipAllPending(batch.id)
  const [progress, setProgress] = useState('')

  return (
    <section className={styles.section} aria-label="Apply the import">
      {pending > 0 ? (
        <p>
          <strong>
            {pending === 1 ? '1 row is' : `${pending} rows are`} waiting for a decision.
          </strong>{' '}
          They have errors the importer could not resolve:
          fix them in the workbook and upload it again, or skip them.{' '}
          <button
            type="button"
            onClick={() => skipAll.mutate((done, total) => setProgress(`Skipping ${done} of ${total}…`))}
            disabled={skipAll.isPending}
          >
            {skipAll.isPending ? progress || 'Skipping…' : pending === 1 ? 'Skip the waiting row' : `Skip all ${pending} waiting rows`}
          </button>
        </p>
      ) : (
        <p>Every row has a decision. Applying adds and updates products in one go: all of it, or nothing.</p>
      )}
      <ErrorText error={skipAll.error} />
      <label className={styles.inline}>
        <input type="checkbox" checked={publicPrices} onChange={(e) => setPublicPrices(e.target.checked)} />
        Show the retail price of new products to customers
      </label>
      <div className={styles.inline}>
        <button
          type="button"
          className="primary"
          disabled={pending > 0 || finish.isPending}
          onClick={() => finish.mutate({ commit: true, retailPricesPublic: publicPrices })}
        >
          {finish.isPending ? 'Applying…' : 'Apply to the catalogue'}
        </button>
        {confirmAbort ? (
          <>
            <span>Discard this import?</span>
            <button type="button" onClick={() => finish.mutate({ commit: false })}>
              Yes, discard
            </button>
            <button type="button" onClick={() => setConfirmAbort(false)}>
              Keep it
            </button>
          </>
        ) : (
          <button type="button" className="link-button" onClick={() => setConfirmAbort(true)}>
            Discard import
          </button>
        )}
      </div>
      <ErrorText error={finish.error} />
    </section>
  )
}

function ImportRowCard({ batchId, row, editable }: { batchId: string; row: ImportRow; editable: boolean }) {
  const decide = useDecideRow(batchId)
  const p = row.proposed
  const hasErrors = row.issues.some((i) => i.severity === 'error')
  const attributes = { ...p.productAttributes, ...p.variantAttributes }
  // The category's form schema turns keys and option values into labels
  // ("Valve Type: A/V"); it is cached, so one request per category.
  const schema = useFormSchema(Object.keys(attributes).length > 0 ? p.categoryId : '').data
  const fieldFor = (key: string) => schema?.fields.find((f) => f.key === key)

  // The decisions that can succeed for this row. The server refuses
  // "accept" for a row with errors and "merge" without a match. A matched
  // row is not offered "accept" either: that would add a second variant
  // with the same SKU to the product, and the whole commit would fail.
  const allowed: ImportDecision[] = p.targetVariantId
    ? ['merge', 'skip']
    : [...(hasErrors ? [] : (['accept'] as const)), 'skip']

  return (
    <li className={styles.importRow}>
      <div className={styles.titleRow}>
        <span className="muted">
          {row.sheet} › row {row.row}
        </span>
        <span className={`badge ${styles[`decision_${row.decision}`] ?? ''}`}>{decisionLabels[row.decision]}</span>
      </div>
      <p>
        <strong>{p.name || '(no name)'}</strong>
        {p.brand && <span className="muted"> · {p.brand}</span>}
        <br />
        <span className="muted">
          SKU {p.sku || '—'}
          {p.supplierItemNo && ` · supplier code ${p.supplierItemNo}`} · stock {p.stockStatus.replace('_', ' ')}
        </span>
      </p>
      {(Object.keys(attributes).length > 0 || p.prices) && (
        <dl className={styles.facts}>
          {Object.entries(attributes).map(([key, value]) => (
            <div key={key}>
              <dt>{fieldFor(key)?.label ?? key}</dt>
              <dd>{formatAttributeValue(fieldFor(key), value)}</dd>
            </div>
          ))}
          {priceTiers
            .filter(({ tier }) => p.prices?.[tier])
            .map(({ tier, label }) => (
              <div key={tier}>
                <dt>{label}</dt>
                <dd>{p.prices?.[tier]}</dd>
              </div>
            ))}
        </dl>
      )}
      {p.targetVariantId && row.targetProductId && (
        <p className="muted">
          Matches an <Link to={`/admin/products/${row.targetProductId}`}>existing product</Link>: “Merge” updates its
          price and stock.
        </p>
      )}
      {row.issues.length > 0 && (
        <ul className={styles.issues}>
          {row.issues.map((issue, i) => (
            <li key={i}>
              <span className={`badge ${styles[`severity_${issue.severity}`] ?? ''}`}>{issue.severity}</span>{' '}
              {issue.message}
            </li>
          ))}
        </ul>
      )}
      <details>
        <summary>Spreadsheet cells</summary>
        <dl className={styles.facts}>
          {Object.entries(row.raw).map(([header, value]) => (
            <div key={header}>
              <dt>{header}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      </details>
      {editable && (
        <div className={styles.inline} role="group" aria-label={`Decision for ${row.sheet} row ${row.row}`}>
          {allowed.map((d) => (
            <button
              key={d}
              type="button"
              aria-pressed={row.decision === d}
              disabled={decide.isPending}
              onClick={() => decide.mutate({ rowId: row.id, decision: d })}
            >
              {decisionLabels[d]}
            </button>
          ))}
        </div>
      )}
      <ErrorText error={decide.error} />
    </li>
  )
}
