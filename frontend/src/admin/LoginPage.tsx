import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router'
import { ApiError } from '../api/client'
import { useLogin, useSession } from '../auth/session'
import { ErrorText } from '../components/ErrorText'

/**
 * Where to go after signing in. Only paths inside the admin are accepted:
 * following any ?next= value would let a crafted link send someone to
 * another site straight after they sign in (an "open redirect").
 */
export function safeNext(next: string | null): string {
  if (next && next.startsWith('/admin') && !next.startsWith('//')) return next
  return '/admin'
}

export function LoginPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const session = useSession()
  const login = useLogin()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const next = safeNext(params.get('next'))

  if (session.data) return <Navigate to={next} replace />

  function submit(e: FormEvent) {
    e.preventDefault()
    login.mutate({ email: email.trim(), password }, { onSuccess: () => navigate(next, { replace: true }) })
  }

  // The API deliberately gives one message for a wrong email or password.
  const error =
    login.error instanceof ApiError && login.error.status === 401
      ? new ApiError(401, 'Wrong email or password.')
      : login.error

  return (
    <main className="content narrow">
      <title>Sign in | Khan's Bike Zone admin</title>
      <h1>Sign in</h1>
      <form className="stack" onSubmit={submit}>
        <label>
          Email
          <input type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label>
          Password
          <input
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        <button type="submit" disabled={login.isPending || !email || !password}>
          {login.isPending ? 'Signing in…' : 'Sign in'}
        </button>
        <ErrorText error={error} />
      </form>
    </main>
  )
}
