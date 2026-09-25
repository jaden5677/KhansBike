import { useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { usePairDevice } from '../api/public'
import { forgetDeviceToken, hasDeviceToken, saveDeviceToken } from '../auth/deviceToken'
import { ErrorText, fieldError } from '../components/ErrorText'

/**
 * Pairs this phone with the admin. The owner shows a QR code in the web
 * admin; scanning it opens /pair?code=ABCD-2345 here. Redeeming the code
 * returns a long-lived token that the phone then uses for admin requests.
 */
export function PairPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const [code, setCode] = useState(params.get('code') ?? '')
  const [name, setName] = useState('')
  const [alreadyPaired, setAlreadyPaired] = useState(hasDeviceToken)
  const pair = usePairDevice()

  function submit(e: FormEvent) {
    e.preventDefault()
    pair.mutate(
      { code: code.trim(), name: name.trim() },
      {
        onSuccess: (result) => {
          saveDeviceToken(result.token)
          navigate('/admin', { replace: true })
        },
      },
    )
  }

  if (alreadyPaired) {
    return (
      <section className="narrow">
        <title>Pair this phone | Khan's Bike Zone</title>
        <h1>This phone is already paired</h1>
        <p>
          <button type="button" onClick={() => navigate('/admin')}>
            Open the admin
          </button>
        </p>
        <p className="muted">
          To pair it again (for example after it was revoked), forget the old pairing first.{' '}
          <button
            type="button"
            className="link-button"
            onClick={() => {
              forgetDeviceToken()
              setAlreadyPaired(false)
            }}
          >
            Forget pairing
          </button>
        </p>
      </section>
    )
  }

  const nameError = fieldError(pair.error, 'name')
  return (
    <section className="narrow">
      <title>Pair this phone | Khan's Bike Zone</title>
      <h1>Pair this phone</h1>
      <p>Enter the code shown in the web admin, and give this phone a name you'll recognise in the device list.</p>
      <form className="stack" onSubmit={submit}>
        <label>
          Pairing code
          <input
            value={code}
            onChange={(e) => setCode(e.target.value)}
            autoComplete="one-time-code"
            autoCapitalize="characters"
            spellCheck={false}
            required
          />
        </label>
        <div className="field">
          <label>
            Name for this phone
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Shop phone"
              required
              aria-invalid={nameError ? true : undefined}
              aria-describedby={nameError ? 'pair-name-error' : undefined}
            />
          </label>
          {nameError && (
            <span id="pair-name-error" className="error-text">
              Name {nameError}
            </span>
          )}
        </div>
        <button type="submit" disabled={pair.isPending || !code.trim() || !name.trim()}>
          {pair.isPending ? 'Pairing…' : 'Pair phone'}
        </button>
        {!nameError && <ErrorText error={pair.error} />}
      </form>
    </section>
  )
}
