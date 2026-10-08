import { useState, type FormEvent } from 'react'
import { useChangePassword } from '../auth/account'
import { ErrorText, fieldError } from '../components/ErrorText'
import styles from './admin.module.css'

/** The server's rule (internal/auth/password.go); checked here only to answer sooner. */
const minPasswordLength = 12

export function AccountPage() {
  const change = useChangePassword()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [problem, setProblem] = useState('')

  function submit(e: FormEvent) {
    e.preventDefault()
    if (next.length < minPasswordLength) {
      setProblem(`The new password needs at least ${minPasswordLength} characters.`)
      return
    }
    if (next !== repeat) {
      setProblem('The two new passwords are not the same.')
      return
    }
    setProblem('')
    change.mutate(
      { currentPassword: current, newPassword: next },
      {
        onSuccess: () => {
          setCurrent('')
          setNext('')
          setRepeat('')
        },
      },
    )
  }

  const currentError = fieldError(change.error, 'currentPassword')
  const newError = fieldError(change.error, 'newPassword')
  return (
    <>
      <title>Account | Khan's Bike Zone admin</title>
      <h1>Account</h1>
      <form onSubmit={submit} className={`stack ${styles.section}`} aria-label="Change password">
        <h2>Change password</h2>
        <p className="muted">
          Use at least {minPasswordLength} characters; a few unrelated words make a strong, memorable password.
          Changing it signs out every other browser.
        </p>
        <div className="field">
          <label>
            Current password
            <input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required />
          </label>
          {currentError && <span className="error-text">Current password {currentError}</span>}
        </div>
        <div className="field">
          <label>
            New password
            <input type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} required />
          </label>
          {newError && <span className="error-text">{newError}</span>}
        </div>
        <label>
          Repeat the new password
          <input type="password" autoComplete="new-password" value={repeat} onChange={(e) => setRepeat(e.target.value)} required />
        </label>
        {problem && (
          <p role="alert" className="error-text">
            {problem}
          </p>
        )}
        {!currentError && !newError && <ErrorText error={change.error} />}
        {change.isSuccess && (
          <p role="status" className="notice">
            Password changed. Other browsers have been signed out.
          </p>
        )}
        <button type="submit" className="primary" disabled={change.isPending || !current || !next || !repeat}>
          {change.isPending ? 'Changing…' : 'Change password'}
        </button>
      </form>
    </>
  )
}
