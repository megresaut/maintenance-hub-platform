import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { fmtCents, get } from '../lib/api'
import type { OutreachThread, WorkOrder } from '../lib/types'
import { Card, EmptyState, ErrorBanner, PageHeader, PriorityBadge, Spinner } from '../components/ui'

type Column = { key: string; label: string; statuses: string[] }

const COLUMNS: Column[] = [
  { key: 'new', label: 'New', statuses: ['new'] },
  { key: 'sourcing', label: 'Sourcing', statuses: ['sent'] },
  { key: 'dispatched', label: 'Dispatched', statuses: ['dispatched', 'scheduled'] },
  { key: 'in_progress', label: 'In Progress', statuses: ['in_progress'] },
  { key: 'done', label: 'Done', statuses: ['completed', 'closed'] },
]

function age(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  const mins = Math.floor(ms / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

export default function Board() {
  const navigate = useNavigate()
  const [workOrders, setWorkOrders] = useState<WorkOrder[] | null>(null)
  const [replyCounts, setReplyCounts] = useState<Map<number, number>>(new Map())
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    get<WorkOrder[]>('/api/work-orders?limit=200')
      .then((wos) => {
        if (!cancelled) setWorkOrders(wos ?? [])
      })
      .catch((e: unknown) => {
        if (cancelled) return
        setWorkOrders([])
        setError(e instanceof Error ? e.message : 'failed to load work orders')
      })
    get<OutreachThread[]>('/api/outreach/all')
      .then((threads) => {
        if (cancelled) return
        const counts = new Map<number, number>()
        for (const t of threads ?? []) {
          const n = t.replies?.length ?? 0
          if (n > 0) counts.set(t.work_order_id, (counts.get(t.work_order_id) ?? 0) + n)
        }
        setReplyCounts(counts)
      })
      .catch(() => {
        /* reply chips are best-effort */
      })
    return () => {
      cancelled = true
    }
  }, [])

  const byColumn = useMemo(() => {
    const map = new Map<string, WorkOrder[]>()
    for (const col of COLUMNS) {
      map.set(
        col.key,
        (workOrders ?? [])
          .filter((w) => col.statuses.includes(w.status))
          .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
      )
    }
    return map
  }, [workOrders])

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Pipeline"
        subtext="Work orders by stage, from intake through completion."
      />
      <ErrorBanner message={error} />

      {workOrders === null ? (
        <Spinner label="Loading pipeline…" />
      ) : workOrders.length === 0 ? (
        <Card>
          <EmptyState title="No work orders yet" hint="Create one from a new request to see it move through the pipeline." />
        </Card>
      ) : (
        <div className="overflow-x-auto pb-2">
          <div className="flex items-start gap-4">
            {COLUMNS.map((col) => {
              const cards = byColumn.get(col.key) ?? []
              return (
                <div key={col.key} className="min-w-64 flex-1">
                  <div className="mb-2 flex items-center gap-2 px-1">
                    <span className="text-sm font-semibold text-slate-700">{col.label}</span>
                    <span className="inline-flex items-center rounded-full bg-slate-200 px-2 py-0.5 text-xs font-medium tabular-nums text-slate-600">
                      {cards.length}
                    </span>
                  </div>
                  <div className="space-y-2 rounded-xl bg-slate-100/70 p-2 min-h-24">
                    {cards.length === 0 ? (
                      <div className="py-6 text-center text-xs text-slate-400">Nothing here</div>
                    ) : (
                      cards.map((w) => {
                        const replies = col.key === 'sourcing' ? (replyCounts.get(w.id) ?? 0) : 0
                        return (
                          <Card
                            key={w.id}
                            className="cursor-pointer px-3.5 py-3 hover:border-sky-300 hover:shadow transition-all"
                          >
                            <div
                              onClick={() => navigate(`/work-orders/${w.id}`)}
                              role="button"
                              tabIndex={0}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter') navigate(`/work-orders/${w.id}`)
                              }}
                            >
                              <div className="text-sm font-medium text-slate-900">{w.name}</div>
                              <div className="mt-0.5 truncate text-xs text-slate-500">
                                {w.property_name ?? '—'}
                              </div>
                              <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                <PriorityBadge priority={w.priority} />
                                {w.category && (
                                  <span className="inline-flex items-center rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                                    {w.category.replace(/_/g, ' ')}
                                  </span>
                                )}
                                {replies > 0 && (
                                  <span className="inline-flex items-center rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                                    💬 {replies} {replies === 1 ? 'reply' : 'replies'}
                                  </span>
                                )}
                              </div>
                              <div className="mt-2 flex items-center justify-between text-xs text-slate-400">
                                <span>{age(w.created_at)}</span>
                                {w.quote_amount_cents != null && (
                                  <span className="font-medium text-slate-600 tabular-nums">
                                    {fmtCents(w.quote_amount_cents)}
                                  </span>
                                )}
                              </div>
                            </div>
                          </Card>
                        )
                      })
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
