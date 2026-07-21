// Thin fetch wrapper. The Vite dev server proxies /api to :8091.

const TOKEN_KEY = 'mh_token'
const SESSION_KEY = 'mh_session'

export type Session = {
  token: string
  org_id: number
  org_name: string
  email: string
  role: string
}

export function getSession(): Session | null {
  const raw = localStorage.getItem(SESSION_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as Session
  } catch {
    return null
  }
}

export function setSession(s: Session | null) {
  if (s) {
    localStorage.setItem(SESSION_KEY, JSON.stringify(s))
    localStorage.setItem(TOKEN_KEY, s.token)
  } else {
    localStorage.removeItem(SESSION_KEY)
    localStorage.removeItem(TOKEN_KEY)
  }
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

// Read-only demo mode (VITE_DEMO=1): the hosted deployment has no Go API or
// Postgres behind it, so GETs are served from a snapshot of the real seeded
// system and writes are declined with a friendly error. Login accepts the
// demo credentials.
export const DEMO = import.meta.env.VITE_DEMO === '1'

// In dev, requests are relative ('/api/...') and the Vite proxy forwards them to
// :8091. In production the web app and Go API are on different origins, so set
// VITE_API_URL to the deployed API origin (no trailing slash) at build time.
const API_BASE = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '')

async function demoApi<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const { default: fixtures } = await import('../demo/fixtures.json')
  const fx = fixtures as Record<string, unknown>
  const method = (opts.method ?? 'GET').toUpperCase()

  if (path === '/api/auth/login' && method === 'POST') {
    return { ...(fx['__login'] as object), token: 'demo-token' } as T
  }
  if (method !== 'GET') {
    throw new ApiError(403, 'This hosted demo is read-only — clone the repo and run it locally for the full interactive pipeline (AI intake, vendor outreach, dispatch).')
  }

  const clean = path.split('#')[0]
  const noQuery = clean.split('?')[0]
  // Exact match (with query), then path-only, then prefix fallbacks for
  // parameterized ranges like /api/schedule?from=...
  for (const key of [clean, noQuery]) {
    if (key in fx) return structuredClone(fx[key]) as T
  }
  if (noQuery.startsWith('/api/schedule')) return structuredClone(fx['/api/schedule']) as T
  if (noQuery.startsWith('/api/drafts') && clean.includes('status=')) {
    return structuredClone(fx['/api/drafts?status=pending']) as T
  }
  throw new ApiError(404, 'not in demo snapshot: ' + path)
}

export async function api<T>(path: string, opts: RequestInit = {}): Promise<T> {
  if (DEMO) return demoApi<T>(path, opts)
  const token = localStorage.getItem(TOKEN_KEY)
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(opts.headers as Record<string, string> | undefined),
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(API_BASE + path, { ...opts, headers })
  if (res.status === 401) {
    setSession(null)
    window.location.href = '/login'
    throw new ApiError(401, 'session expired')
  }
  if (!res.ok) {
    let msg = res.statusText
    try {
      const body = await res.json()
      if (body.error) msg = body.error
    } catch {
      /* not json */
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const get = <T,>(path: string) => api<T>(path)
export const post = <T,>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: body === undefined ? '{}' : JSON.stringify(body) })
export const put = <T,>(path: string, body?: unknown) =>
  api<T>(path, { method: 'PUT', body: JSON.stringify(body ?? {}) })
export const del = <T,>(path: string) => api<T>(path, { method: 'DELETE' })

export function fmtCents(cents?: number | null): string {
  if (cents === null || cents === undefined) return '—'
  return `$${(cents / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}
