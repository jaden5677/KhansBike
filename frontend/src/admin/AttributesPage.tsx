import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import type { DataType } from '../api/types'
import { useAdminAttributes, useSaveAttribute } from '../api/adminTaxonomy'
import { ErrorText, fieldError } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'

export const dataTypes: { type: DataType; label: string; example: string }[] = [
  { type: 'enum', label: 'One choice from a list', example: 'Valve type: Presta' },
  { type: 'multi_enum', label: 'Several choices from a list', example: 'Riding style: Road, Gravel' },
  { type: 'color', label: 'Colour', example: 'Black (with a swatch)' },
  { type: 'number', label: 'Number', example: 'Spokes: 36' },
  { type: 'number_range', label: 'Range of numbers', example: 'Tyre width: 1.95–2.125' },
  { type: 'boolean', label: 'Yes or no', example: 'Tubeless ready' },
  { type: 'text', label: 'Free text', example: 'Material: Aluminium' },
]

/** "Wheel size" → "wheel_size": the key format the API requires. */
export function keyFromLabel(label: string): string {
  return label
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .replace(/^(\d)/, 'a_$1')
    .slice(0, 63)
}

export function AttributesPage() {
  const attributes = useAdminAttributes()
  const save = useSaveAttribute()
  const navigate = useNavigate()
  const [label, setLabel] = useState('')
  const [key, setKey] = useState('')
  const [keyEdited, setKeyEdited] = useState(false)
  const [dataType, setDataType] = useState<DataType>('enum')
  const [unit, setUnit] = useState('')

  function create(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      {
        input: {
          key: key.trim(),
          label: label.trim(),
          dataType,
          unit: unit.trim() || null,
          isFilterable: true,
          isSearchable: false,
          helpText: null,
        },
      },
      // Straight to the new attribute, where its options are added.
      { onSuccess: (saved) => navigate(`/admin/attributes/${saved.data.id}`) },
    )
  }

  return (
    <>
      <title>Attributes | Khan's Bike Zone admin</title>
      <h1>Attributes</h1>
      <p className="muted">
        The details products can have, such as wheel size or colour. Attach them to categories to use them.
      </p>
      <Loadable query={attributes}>
        {(list) => (
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Name</th>
                <th>Key</th>
                <th>Type</th>
                <th>Options</th>
              </tr>
            </thead>
            <tbody>
              {list.map((a) => (
                <tr key={a.id}>
                  <td>
                    <Link to={`/admin/attributes/${a.id}`}>{a.label}</Link>
                  </td>
                  <td>
                    <code>{a.key}</code>
                  </td>
                  <td>{dataTypes.find((d) => d.type === a.dataType)?.label ?? a.dataType}</td>
                  <td>{a.options.length || ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Loadable>

      <form onSubmit={create} className={`stack ${styles.section}`} aria-label="New attribute">
        <h2>New attribute</h2>
        <div className="field">
          <label>
            Name
            <input
              value={label}
              onChange={(e) => {
                setLabel(e.target.value)
                if (!keyEdited) setKey(keyFromLabel(e.target.value))
              }}
              placeholder="e.g. Valve type"
            />
          </label>
          {fieldError(save.error, 'label') && <span className="error-text">{fieldError(save.error, 'label')}</span>}
        </div>
        <div className="field">
          <label>
            Key
            <input
              value={key}
              onChange={(e) => {
                setKey(e.target.value)
                setKeyEdited(true)
              }}
              spellCheck={false}
            />
          </label>
          <span className="muted">Used in filter links; it cannot be changed later.</span>
          {fieldError(save.error, 'key') && <span className="error-text">{fieldError(save.error, 'key')}</span>}
        </div>
        <label>
          Type (cannot be changed later)
          <select value={dataType} onChange={(e) => setDataType(e.target.value as DataType)}>
            {dataTypes.map((d) => (
              <option key={d.type} value={d.type}>
                {d.label} — e.g. {d.example}
              </option>
            ))}
          </select>
        </label>
        {(dataType === 'number' || dataType === 'number_range') && (
          <label>
            Unit
            <input value={unit} onChange={(e) => setUnit(e.target.value)} placeholder="e.g. in, mm" />
          </label>
        )}
        <button type="submit" disabled={save.isPending || !label.trim() || !key.trim()}>
          Create attribute
        </button>
        {!fieldError(save.error, 'label') && !fieldError(save.error, 'key') && <ErrorText error={save.error} />}
      </form>
    </>
  )
}
