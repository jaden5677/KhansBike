import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import type { Versioned } from '../api/admin'
import type { AdminAttribute, AdminOption } from '../api/adminTypes'
import {
  useAdminAttribute,
  useDeleteAttribute,
  useDeleteOption,
  useSaveAttribute,
  useSaveOption,
} from '../api/adminTaxonomy'
import { ApiError } from '../api/client'
import { ErrorText, fieldError } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'
import { dataTypes } from './AttributesPage'

const hasOptions = (a: AdminAttribute) => ['enum', 'multi_enum', 'color'].includes(a.dataType)

/** "26 inch" → "26-inch": the stable value stored on products and used in filter links. */
export function optionValue(label: string): string {
  return label
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function AttributeEditPage() {
  const { id = '' } = useParams()
  const attribute = useAdminAttribute(id)
  return (
    <>
      <p>
        <Link to="/admin/attributes">← Attributes</Link>
      </p>
      <Loadable query={attribute}>
        {(loaded) => (
          <>
            <AttributeForm key={loaded.data.id} loaded={loaded} />
            {hasOptions(loaded.data) && <Options attribute={loaded.data} />}
          </>
        )}
      </Loadable>
    </>
  )
}

function AttributeForm({ loaded }: { loaded: Versioned<AdminAttribute> }) {
  const a = loaded.data
  const navigate = useNavigate()
  const save = useSaveAttribute()
  const remove = useDeleteAttribute()
  const [label, setLabel] = useState(a.label)
  const [unit, setUnit] = useState(a.unit ?? '')
  const [helpText, setHelpText] = useState(a.helpText ?? '')
  const [isFilterable, setIsFilterable] = useState(a.isFilterable)
  const [isSearchable, setIsSearchable] = useState(a.isSearchable)

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate({
      id: a.id,
      etag: loaded.etag,
      input: {
        label: label.trim(),
        unit: unit.trim() || null,
        helpText: helpText.trim() || null,
        inputType: a.inputType,
        isFilterable,
        isSearchable,
      },
    })
  }

  return (
    <>
      <title>{`${a.label} | Attributes | Khan's Bike Zone admin`}</title>
      <h1>{a.label}</h1>
      <p className="muted">
        Key <code>{a.key}</code> · {dataTypes.find((d) => d.type === a.dataType)?.label}. The key and type are fixed:
        saved product values and filter links depend on them.
      </p>
      <form onSubmit={submit} className={`stack ${styles.section}`}>
        <div className="field">
          <label>
            Name
            <input value={label} onChange={(e) => setLabel(e.target.value)} />
          </label>
          {fieldError(save.error, 'label') && <span className="error-text">{fieldError(save.error, 'label')}</span>}
        </div>
        <label>
          Unit
          <input value={unit} onChange={(e) => setUnit(e.target.value)} placeholder="e.g. in, mm" />
        </label>
        <div className="field field-wide">
          <label>
            Help text for the product form
            <input value={helpText} onChange={(e) => setHelpText(e.target.value)} />
          </label>
        </div>
        <div className="checkbox-list">
          <label>
            <input type="checkbox" checked={isFilterable} onChange={(e) => setIsFilterable(e.target.checked)} />
            Customers can filter by it
          </label>
          <label>
            <input type="checkbox" checked={isSearchable} onChange={(e) => setIsSearchable(e.target.checked)} />
            Search matches its values
          </label>
        </div>
        {save.error instanceof ApiError && save.error.status === 412 ? (
          <p role="alert" className="notice">
            Someone else changed this attribute. Reload the page to see their version.
          </p>
        ) : (
          !fieldError(save.error, 'label') && <ErrorText error={save.error} />
        )}
        {save.isSuccess && <p role="status">Saved.</p>}
        <div className={styles.inline}>
          <button type="submit" className="primary" disabled={save.isPending}>
            Save attribute
          </button>
          <button
            type="button"
            className="link-button"
            onClick={() => remove.mutate(a.id, { onSuccess: () => navigate('/admin/attributes', { replace: true }) })}
          >
            Delete attribute
          </button>
        </div>
        {/* e.g. 409 while a category still uses it */}
        <ErrorText error={remove.error} />
      </form>
    </>
  )
}

/** The choices of a list-type attribute. An option's value is fixed once created. */
function Options({ attribute }: { attribute: AdminAttribute }) {
  const save = useSaveOption(attribute.id)
  const remove = useDeleteOption(attribute.id)
  const [label, setLabel] = useState('')
  const [swatch, setSwatch] = useState('#000000')
  const isColour = attribute.dataType === 'color'
  const sorted = [...attribute.options].sort((a, b) => a.position - b.position)

  function add(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      {
        input: {
          value: optionValue(label),
          label: label.trim(),
          swatchHex: isColour ? swatch : null,
          position: sorted.length,
        },
      },
      { onSuccess: () => setLabel('') },
    )
  }

  return (
    <section className={styles.section}>
      <h2>Options</h2>
      {sorted.length === 0 && <p className="muted">No options yet.</p>}
      <ul className={styles.options}>
        {sorted.map((o) => (
          <OptionRow key={o.id} option={o} isColour={isColour} attributeId={attribute.id} onDelete={() => remove.mutate(o.id)} />
        ))}
      </ul>
      <form onSubmit={add} className={styles.inline} aria-label="Add an option">
        <label>
          New option{' '}
          <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="e.g. Presta" />
        </label>
        {isColour && (
          <label>
            Swatch <input type="color" value={swatch} onChange={(e) => setSwatch(e.target.value)} />
          </label>
        )}
        <button type="submit" disabled={save.isPending || !label.trim()}>
          Add option
        </button>
      </form>
      {/* e.g. 409 when products still use an option being deleted */}
      <ErrorText error={save.error ?? remove.error} />
    </section>
  )
}

function OptionRow({
  option,
  isColour,
  attributeId,
  onDelete,
}: {
  option: AdminOption
  isColour: boolean
  attributeId: string
  onDelete: () => void
}) {
  const save = useSaveOption(attributeId)
  const [label, setLabel] = useState(option.label)
  const [swatch, setSwatch] = useState(option.swatchHex ?? '#000000')
  const changed = label !== option.label || (isColour && swatch !== (option.swatchHex ?? '#000000'))
  return (
    <li className={styles.inline}>
      <input aria-label={`Label for ${option.value}`} value={label} onChange={(e) => setLabel(e.target.value)} />
      {isColour && (
        <input type="color" aria-label={`Swatch for ${option.value}`} value={swatch} onChange={(e) => setSwatch(e.target.value)} />
      )}
      <code className="muted">{option.value}</code>
      {changed && (
        <button
          type="button"
          onClick={() =>
            save.mutate({
              optionId: option.id,
              input: { label: label.trim(), swatchHex: isColour ? swatch : null, position: option.position },
            })
          }
        >
          Save
        </button>
      )}
      <button type="button" className="link-button" onClick={onDelete}>
        Delete
      </button>
      <ErrorText error={save.error} />
    </li>
  )
}
