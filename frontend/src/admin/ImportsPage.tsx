import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import type { ImportBatch, ImportBatchStatus } from '../api/adminTypes'
import { useImportBatches, useStageWorkbook } from '../api/adminImports'
import { ErrorText } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'

export const batchStatusLabels: Record<ImportBatchStatus, string> = {
  dry_run: 'Ready for review',
  reviewing: 'In review',
  committed: 'Applied',
  aborted: 'Discarded',
}

const dateTime = new Intl.DateTimeFormat('en-TT', { dateStyle: 'medium', timeStyle: 'short' })

/** "12 waiting · 80 accept · 3 merge · 1 skip" */
export function countsSummary(b: ImportBatch): string {
  const c = b.counts ?? {}
  return [
    [c.pending, 'waiting'],
    [c.accept, 'accept'],
    [c.merge, 'merge'],
    [c.skip, 'skip'],
  ]
    .filter(([n]) => n)
    .map(([n, label]) => `${n} ${label}`)
    .join(' · ')
}

/** The price-list importer: upload a workbook, then review it before anything changes. */
export function ImportsPage() {
  const batches = useImportBatches()
  const stage = useStageWorkbook()
  const navigate = useNavigate()
  const [file, setFile] = useState<File | null>(null)

  function upload(e: FormEvent) {
    e.preventDefault()
    if (file) stage.mutate(file, { onSuccess: (batch) => navigate(`/admin/imports/${batch.id}`) })
  }

  return (
    <>
      <title>Imports | Khan's Bike Zone admin</title>
      <h1>Import the price list</h1>
      <form onSubmit={upload} className={`stack ${styles.section}`} aria-label="Upload a workbook">
        <p>
          Upload the price-list workbook (.xlsx). Each sheet is matched to the category with the same name. Nothing
          in the shop changes until you review the rows and apply them.
        </p>
        <label>
          Workbook
          <input
            type="file"
            accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </label>
        <button type="submit" disabled={!file || stage.isPending}>
          {stage.isPending ? 'Reading the workbook…' : 'Upload and review'}
        </button>
        <ErrorText error={stage.error} />
      </form>

      <h2>Earlier imports</h2>
      <Loadable query={batches}>
        {(list) =>
          list.length === 0 ? (
            <p className="muted">None yet.</p>
          ) : (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Workbook</th>
                  <th>Status</th>
                  <th>Rows</th>
                  <th>Uploaded</th>
                </tr>
              </thead>
              <tbody>
                {list.map((b) => (
                  <tr key={b.id}>
                    <td>
                      <Link to={`/admin/imports/${b.id}`}>{b.filename}</Link>
                    </td>
                    <td>{batchStatusLabels[b.status]}</td>
                    <td>{countsSummary(b)}</td>
                    <td>{dateTime.format(new Date(b.createdAt))}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        }
      </Loadable>
    </>
  )
}
