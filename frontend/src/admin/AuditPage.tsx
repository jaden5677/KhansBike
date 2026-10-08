import { Link, useSearchParams } from 'react-router'
import { useAuditLog } from '../api/admin'
import type { AuditEntry } from '../api/adminTypes'
import { Loadable } from '../components/Loadable'
import { changedFields, describeAction, entityLink } from '../lib/audit'
import { withParam } from '../lib/filters'
import styles from './admin.module.css'

const entityTypes: { type: string; label: string }[] = [
  { type: 'product', label: 'Products' },
  { type: 'category', label: 'Categories' },
  { type: 'attribute', label: 'Attributes' },
  { type: 'brand', label: 'Brands' },
  { type: 'supplier', label: 'Suppliers' },
  { type: 'import_batch', label: 'Price-list imports' },
  { type: 'media_asset', label: 'Image uploads' },
  { type: 'device', label: 'Phones' },
  { type: 'user', label: 'Sign-ins and passwords' },
]

const dateTime = new Intl.DateTimeFormat('en-TT', { dateStyle: 'medium', timeStyle: 'short' })

function who(e: AuditEntry): string {
  if (e.actor.kind === 'system') return 'System'
  const name = e.actor.email ?? 'Unknown user'
  return e.actor.kind === 'device' ? `${name} (phone)` : name
}

/** Every change made in the admin: who, what and when, newest first. */
export function AuditPage() {
  const [params, setParams] = useSearchParams()
  const type = params.get('type') ?? ''
  const log = useAuditLog(type)

  return (
    <>
      <title>Activity | Khan's Bike Zone admin</title>
      <h1>Activity</h1>
      <p className="muted">Every change made in the admin, newest first. It cannot be edited or deleted.</p>
      <label>
        Show{' '}
        <select value={type} onChange={(e) => setParams(withParam(params, 'type', e.target.value || undefined))}>
          <option value="">Everything</option>
          {entityTypes.map((t) => (
            <option key={t.type} value={t.type}>
              {t.label}
            </option>
          ))}
        </select>
      </label>
      <Loadable query={log}>
        {(data) => {
          const entries = data.pages.flatMap((p) => p.items)
          if (entries.length === 0) return <p className="muted">Nothing yet.</p>
          return (
            <>
              <ul className={styles.audit}>
                {entries.map((e) => (
                  <AuditRow key={e.id} entry={e} />
                ))}
              </ul>
              {log.hasNextPage && (
                <p className="load-more">
                  <button type="button" onClick={() => log.fetchNextPage()} disabled={log.isFetchingNextPage}>
                    {log.isFetchingNextPage ? 'Loading…' : 'Show older'}
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

function AuditRow({ entry }: { entry: AuditEntry }) {
  const link = entityLink(entry.entityType, entry.entityId)
  const changes = changedFields(entry.before, entry.after)
  return (
    <li>
      <div className={styles.inline}>
        <time dateTime={entry.createdAt} className="muted">
          {dateTime.format(new Date(entry.createdAt))}
        </time>
        <strong>{describeAction(entry.action)}</strong>
        {link && <Link to={link}>open</Link>}
        <span className="muted">by {who(entry)}</span>
      </div>
      {changes.length > 0 && (
        <details>
          <summary>
            What changed ({changes.length} {changes.length === 1 ? 'field' : 'fields'})
          </summary>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Field</th>
                <th>Before</th>
                <th>After</th>
              </tr>
            </thead>
            <tbody>
              {changes.map((c) => (
                <tr key={c.field}>
                  <td>{c.field}</td>
                  <td>{c.before}</td>
                  <td>{c.after}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      )}
    </li>
  )
}
