import { useEffect, useState } from 'react'
import type { Device, PairingCode } from '../api/adminTypes'
import { useCreatePairingCode, useDevices, useRevokeDevice } from '../auth/account'
import { ErrorText } from '../components/ErrorText'
import { Loadable } from '../components/Loadable'
import { QRCode } from '../components/QRCode'
import styles from './admin.module.css'

const dateTime = new Intl.DateTimeFormat('en-TT', { dateStyle: 'medium', timeStyle: 'short' })

/** "ABCD2345" → "ABCD-2345", easier to read out and type. */
export function formatCode(code: string): string {
  return code.length === 8 ? `${code.slice(0, 4)}-${code.slice(4)}` : code
}

/** Seconds left until `at`, updated every second (never below zero). */
function useSecondsUntil(at: string): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])
  return Math.max(0, Math.round((new Date(at).getTime() - now) / 1000))
}

/** Paired phones: pair a new one with a QR code, see and revoke existing ones. */
export function DevicesPage() {
  const pairing = useCreatePairingCode()
  const code = pairing.data
  // While a code is on screen, check for the new phone every few seconds.
  const devices = useDevices(code ? 5000 : false)

  return (
    <>
      <title>Phones | Khan's Bike Zone admin</title>
      <h1>Phones</h1>
      <section className={styles.section}>
        <h2>Pair a phone</h2>
        <p>
          A paired phone can manage the catalogue (take photos, update stock) without signing in. Show a code, then
          scan it with the phone's camera.
        </p>
        {code ? (
          <PairingPanel code={code} onNewCode={() => pairing.mutate()} />
        ) : (
          <button type="button" onClick={() => pairing.mutate()} disabled={pairing.isPending}>
            Show a pairing code
          </button>
        )}
        <ErrorText error={pairing.error} />
      </section>

      <section className={styles.section}>
        <h2>Paired phones</h2>
        <Loadable query={devices}>
          {(list) =>
            list.length === 0 ? (
              <p className="muted">No phones paired yet.</p>
            ) : (
              <ul className={styles.options}>
                {list.map((d) => (
                  <DeviceRow key={d.id} device={d} />
                ))}
              </ul>
            )
          }
        </Loadable>
      </section>
    </>
  )
}

function PairingPanel({ code, onNewCode }: { code: PairingCode; onNewCode: () => void }) {
  const seconds = useSecondsUntil(code.expiresAt)
  if (seconds === 0) {
    return (
      <p>
        That code has expired.{' '}
        <button type="button" onClick={onNewCode}>
          Show a new code
        </button>
      </p>
    )
  }
  return (
    <div className={styles.pairing}>
      <QRCode value={code.url} label={`Pairing QR code for ${code.url}`} />
      <div className="stack">
        <p>
          Code <strong className={styles.code}>{formatCode(code.code)}</strong>
        </p>
        <p className="muted">
          Or open <a href={code.url}>{code.url}</a> on the phone. The code works once and expires in{' '}
          {Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, '0')}.
        </p>
      </div>
    </div>
  )
}

function DeviceRow({ device }: { device: Device }) {
  const revoke = useRevokeDevice()
  const [confirm, setConfirm] = useState(false)
  return (
    <li className={styles.inline}>
      <strong>{device.name}</strong>
      <span className="muted">
        paired {dateTime.format(new Date(device.createdAt))}
        {device.lastSeenAt && ` · last used ${dateTime.format(new Date(device.lastSeenAt))}`}
      </span>
      {device.revokedAt ? (
        <span className="badge">revoked {dateTime.format(new Date(device.revokedAt))}</span>
      ) : confirm ? (
        <>
          <span>It will be signed out at once.</span>
          <button type="button" onClick={() => revoke.mutate(device.id)} disabled={revoke.isPending}>
            Revoke
          </button>
          <button type="button" onClick={() => setConfirm(false)}>
            Cancel
          </button>
        </>
      ) : (
        <button type="button" className="link-button" onClick={() => setConfirm(true)}>
          Revoke {device.name}
        </button>
      )}
      <ErrorText error={revoke.error} />
    </li>
  )
}
