import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { get, fmtCents, fmtDate } from '../lib/api'
import {
  FTO_STATUSES,
  TASK_STATUSES,
  WO_STATUSES,
  type AIDraft,
  type FTO,
  type Property,
  type Task,
  type WorkOrder,
} from '../lib/types'
import { Card, EmptyState, ErrorBanner, PageHeader, PriorityBadge, Spinner, StatusBadge, inputCls } from '../components/ui'

type Tab = 'wo' | 'fto' | 'task'

const TABS: { key: Tab; label: string }[] = [
  { key: 'wo', label: 'Work Orders' },
  { key: 'fto', label: 'Internal (FTO)' },
  { key: 'task', label: 'Tasks' },
]

function StatTile({ label, value, accent }: { label: string; value: number | string; accent?: string }) {
  return (
    <Card className="px-5 py-4">
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-1 text-3xl font-semibold tabular-nums ${accent ?? 'text-slate-900'}`}>{value}</div>
    </Card>
  )
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [workOrders, setWorkOrders] = useState<WorkOrder[] | null>(null)
  const [ftos, setFtos] = useState<FTO[] | null>(null)
  const [tasks, setTasks] = useState<Task[] | null>(null)
  const [pendingDrafts, setPendingDrafts] = useState<AIDraft[] | null>(null)
  const [properties, setProperties] = useState<Property[]>([])
  const [error, setError] = useState('')

  const [tab, setTab] = useState<Tab>('wo')
  const [propertyFilter, setPropertyFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [query, setQuery] = useState('')

  useEffect(() => {
    let cancelled = false
    Promise.all([
      get<WorkOrder[]>('/api/work-orders?limit=200'),
      get<FTO[]>('/api/ftos?limit=200'),
      get<Task[]>('/api/tasks?limit=200'),
      get<AIDraft[]>('/api/drafts?status=pending'),
      get<Property[]>('/api/properties'),
    ])
      .then(([wos, fs, ts, drafts, props]) => {
        if (cancelled) return
        setWorkOrders(wos ?? [])
        setFtos(fs ?? [])
        setTasks(ts ?? [])
        setPendingDrafts(drafts ?? [])
        setProperties(props ?? [])
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : 'failed to load dashboard')
      })
    return () => {
      cancelled = true
    }
  }, [])

  const loading = workOrders === null || ftos === null || tasks === null || pendingDrafts === null

  const openWOs = (workOrders ?? []).filter((w) => !['completed', 'cancelled', 'closed'].includes(w.status))
  const sourcing = (workOrders ?? []).filter((w) => w.status === 'sent')
  const openFtos = (ftos ?? []).filter((f) => !['completed', 'cancelled'].includes(f.status))
  const pendingTopLevel = (pendingDrafts ?? []).filter((d) => !d.parent_draft_id)

  const statusOptions = tab === 'wo' ? WO_STATUSES : tab === 'fto' ? FTO_STATUSES : TASK_STATUSES

  type Row = {
    id: number
    name: string
    property_name?: string | null
    property_id?: number | null
    status: string
    priority: string
    quote_amount_cents?: number | null
    created_at: string
  }

  const rows: Row[] = useMemo(() => {
    const src: Row[] = tab === 'wo' ? (workOrders ?? []) : tab === 'fto' ? (ftos ?? []) : (tasks ?? [])
    const q = query.trim().toLowerCase()
    return src.filter((r) => {
      if (propertyFilter && String(r.property_id ?? '') !== propertyFilter) return false
      if (statusFilter && r.status !== statusFilter) return false
      if (q && !r.name.toLowerCase().includes(q)) return false
      return true
    })
  }, [tab, workOrders, ftos, tasks, propertyFilter, statusFilter, query])

  const rowHref = (r: Row) =>
    tab === 'wo' ? `/work-orders/${r.id}` : tab === 'fto' ? `/tickets/fto/${r.id}` : `/tickets/task/${r.id}`

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Dashboard"
        subtext="Portfolio-wide view of every open ticket across your properties."
      />
      <ErrorBanner message={error} />

      {loading ? (
        <Spinner />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatTile label="Open Work Orders" value={openWOs.length} accent="text-sky-600" />
            <StatTile label="Sourcing Vendors" value={sourcing.length} accent="text-indigo-600" />
            <StatTile label="Pending Drafts" value={pendingTopLevel.length} accent="text-amber-600" />
            <StatTile label="Open Internal" value={openFtos.length} accent="text-emerald-600" />
          </div>

          <Card className="mt-6 overflow-hidden">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-5 pt-3">
              <div className="flex gap-1">
                {TABS.map((t) => (
                  <button
                    key={t.key}
                    onClick={() => {
                      setTab(t.key)
                      setStatusFilter('')
                    }}
                    className={`border-b-2 px-3 pb-3 pt-1 text-sm font-medium transition-colors ${
                      tab === t.key
                        ? 'border-sky-600 text-sky-700'
                        : 'border-transparent text-slate-500 hover:text-slate-700'
                    }`}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
              <div className="flex flex-wrap items-center gap-2 pb-3">
                <select
                  value={propertyFilter}
                  onChange={(e) => setPropertyFilter(e.target.value)}
                  className={`${inputCls} w-44`}
                >
                  <option value="">All properties</option>
                  {properties.map((p) => (
                    <option key={p.id} value={String(p.id)}>
                      {p.name}
                    </option>
                  ))}
                </select>
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className={`${inputCls} w-40`}
                >
                  <option value="">All statuses</option>
                  {statusOptions.map((s) => (
                    <option key={s} value={s}>
                      {s === 'sent' && tab === 'wo' ? 'sourcing' : s.replace(/_/g, ' ')}
                    </option>
                  ))}
                </select>
                <input
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search by name…"
                  className={`${inputCls} w-52`}
                />
              </div>
            </div>

            {rows.length === 0 ? (
              <EmptyState title="No tickets match" hint="Try clearing filters or create a new request." />
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-200 text-left text-xs font-medium uppercase tracking-wide text-slate-500">
                    <th className="px-5 py-3">Name</th>
                    <th className="px-5 py-3">Property</th>
                    <th className="px-5 py-3">Status</th>
                    <th className="px-5 py-3">Priority</th>
                    {tab === 'wo' && <th className="px-5 py-3">Quote</th>}
                    <th className="px-5 py-3">Created</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr
                      key={r.id}
                      onClick={() => navigate(rowHref(r))}
                      className="cursor-pointer border-b border-slate-100 last:border-0 hover:bg-sky-50/40 transition-colors"
                    >
                      <td className="px-5 py-3 font-medium text-slate-900">{r.name}</td>
                      <td className="px-5 py-3 text-slate-600">{r.property_name ?? '—'}</td>
                      <td className="px-5 py-3">
                        <StatusBadge status={r.status} />
                      </td>
                      <td className="px-5 py-3">
                        <PriorityBadge priority={r.priority} />
                      </td>
                      {tab === 'wo' && (
                        <td className="px-5 py-3 tabular-nums text-slate-700">{fmtCents(r.quote_amount_cents)}</td>
                      )}
                      <td className="px-5 py-3 text-slate-500 whitespace-nowrap">{fmtDate(r.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>
        </>
      )}
    </div>
  )
}
