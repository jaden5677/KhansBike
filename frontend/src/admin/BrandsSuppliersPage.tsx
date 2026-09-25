import { useState, type FormEvent } from 'react'
import { useAdminBrands, useAdminSuppliers } from '../api/admin'
import type { AdminBrand, AdminSupplier } from '../api/adminTypes'
import { useDeleteBrand, useDeleteSupplier, useSaveBrand, useSaveSupplier } from '../api/adminTaxonomy'
import { ErrorText } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import styles from './admin.module.css'

export function BrandsSuppliersPage() {
  return (
    <>
      <title>Brands and suppliers | Khan's Bike Zone admin</title>
      <h1>Brands and suppliers</h1>
      <Brands />
      <Suppliers />
    </>
  )
}

function Brands() {
  const brands = useAdminBrands()
  const save = useSaveBrand()
  const [name, setName] = useState('')

  function add(e: FormEvent) {
    e.preventDefault()
    save.mutate({ input: { name: name.trim(), position: 0 } }, { onSuccess: () => setName('') })
  }

  return (
    <section className={styles.section}>
      <h2>Brands</h2>
      <p className="muted">Shown to customers on products and the brands page.</p>
      <Loadable query={brands}>
        {(list) => (
          <ul className={styles.options}>
            {list.map((b) => (
              <BrandRow key={b.id} brand={b} />
            ))}
          </ul>
        )}
      </Loadable>
      <form onSubmit={add} className={styles.inline} aria-label="Add a brand">
        <label>
          New brand <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <button type="submit" disabled={save.isPending || !name.trim()}>
          Add brand
        </button>
      </form>
      <ErrorText error={save.error} />
    </section>
  )
}

function BrandRow({ brand }: { brand: AdminBrand }) {
  const save = useSaveBrand()
  const remove = useDeleteBrand()
  const [name, setName] = useState(brand.name)
  return (
    <li className={styles.inline}>
      <input aria-label={`Name of ${brand.name}`} value={name} onChange={(e) => setName(e.target.value)} />
      {name.trim() !== brand.name && (
        <button
          type="button"
          onClick={() => save.mutate({ id: brand.id, input: { name: name.trim(), slug: brand.slug, position: brand.position } })}
        >
          Save
        </button>
      )}
      <button type="button" className="link-button" onClick={() => remove.mutate(brand.id)}>
        Delete
      </button>
      <ErrorText error={save.error ?? remove.error} />
    </li>
  )
}

function Suppliers() {
  const suppliers = useAdminSuppliers()
  const save = useSaveSupplier()
  const [name, setName] = useState('')
  const [code, setCode] = useState('')

  function add(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      { input: { name: name.trim(), code: code.trim() || null, notes: null } },
      {
        onSuccess: () => {
          setName('')
          setCode('')
        },
      },
    )
  }

  return (
    <section className={styles.section}>
      <h2>Suppliers</h2>
      <p className="muted">Only ever shown in the admin.</p>
      <Loadable query={suppliers}>
        {(list) => (
          <ul className={styles.options}>
            {list.map((s) => (
              <SupplierRow key={s.id} supplier={s} />
            ))}
          </ul>
        )}
      </Loadable>
      <form onSubmit={add} className={styles.inline} aria-label="Add a supplier">
        <label>
          New supplier <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          Code <input value={code} onChange={(e) => setCode(e.target.value)} size={8} />
        </label>
        <button type="submit" disabled={save.isPending || !name.trim()}>
          Add supplier
        </button>
      </form>
      <ErrorText error={save.error} />
    </section>
  )
}

function SupplierRow({ supplier }: { supplier: AdminSupplier }) {
  const save = useSaveSupplier()
  const remove = useDeleteSupplier()
  const [name, setName] = useState(supplier.name)
  const [code, setCode] = useState(supplier.code ?? '')
  const changed = name.trim() !== supplier.name || code.trim() !== (supplier.code ?? '')
  return (
    <li className={styles.inline}>
      <input aria-label={`Name of ${supplier.name}`} value={name} onChange={(e) => setName(e.target.value)} />
      <input aria-label={`Code of ${supplier.name}`} value={code} onChange={(e) => setCode(e.target.value)} size={8} />
      {changed && (
        <button
          type="button"
          onClick={() =>
            save.mutate({ id: supplier.id, input: { name: name.trim(), code: code.trim() || null, notes: supplier.notes } })
          }
        >
          Save
        </button>
      )}
      <button type="button" className="link-button" onClick={() => remove.mutate(supplier.id)}>
        Delete
      </button>
      <ErrorText error={save.error ?? remove.error} />
    </li>
  )
}
