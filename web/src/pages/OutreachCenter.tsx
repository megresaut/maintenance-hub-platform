import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { fmtCents, fmtDate, get } from '../lib/api'
import type { OutreachReply, OutreachThread } from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
  PageHeader,
  Spinner,
  StatusBadge,
  inputCls,
} from '../components/ui'

const POLL_MS = 10_000

type StatusFilter = '' | 'awaiting' | 'replied' | 'won' | 'failed'

function StatTile({ label, value, accent }: { label: string; value: number | string; accent?: string }) {
  return (
    <Card className="px-5 py-4">
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</div>
      <div className={`mt-1 text-3xl font-semibold tabular-nums ${accent ?? 'text-slate-900'}`}>{value}</div>
    </Card>
  )
}

function RequestStatusBadge({ t }: { t: OutreachThread }) {
  switch (t.status) {
    case 'selected':
      return <StatusBadge status="selected" label="Won" />
    case 'replied':
      return <StatusBadge status="replied" />
    case 'sent':
      // "pending" style gives the amber "waiting" look.
      return <StatusBadge status="pending" label="Awaiting" />
    case 'failed':
      return (
        <span title={t.error ?? undefined}>
          <StatusBadge status="failed" />
        </span>
      )
    default:
      return <StatusBadge status="pending" label="Pending" />
  }
}

function ChannelChip({ channel }: { channel: 'sms' | 'email' }) {
  return (
    <span className="inline-flex items-center rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-500">
      {channel === 'sms' ? 'SMS' : 'Email'}
    </span>
  )
}

function ReplyBlock({ reply }: { reply: OutreachReply }) {
  const [expanded, setExpanded] = useState(false)
  const long = reply.body.length > 140
  const text = long && !expanded ? `${reply.body.slice(0, 140)}…` : reply.body
  return (
    <div className="mt-2 rounded-md border-l-2 border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
      <span className="whitespace-pre-wrap">{text}</span>
      {long && (
        <button
          onClick={() => setExpanded((e) => !e)}
          className="ml-2 text-xs font-medium text-sky-600 hover:text-sky-700"
        >
          {expanded ? 'Show less' : 'Show more'}
        </button>
      )}
      <div className="mt-1 text-xs text-slate-400">Replied {fmtDate(reply.received_at)}</div>
    </div>
  )
}

function latestReply(t: OutreachThread): OutreachReply | null {
  const rs = t.replies ?? []
  if (rs.length === 0) return null
  return [...rs].sort((a, b) => new Date(b.received_at).getTime() - new Date(a.received_at).getTime())[0]
}

function latestQuote(t: OutreachThread): OutreachReply | null {
  const rs = (t.replies ?? []).filter((r) => r.parsed_quote_cents != null || r.parsed_availability != null)
  if (rs.length === 0) return null
  return [...rs].sort((a, b) => new Date(b.received_at).getTime() - new Date(a.received_at).getTime())[0]
}

function groupActivity(rows: OutreachThread[]): number {
  let max = 0
  for (const r of rows) {
    for (const ts of [r.latest_reply_at, r.sent_at, r.created_at]) {
      if (ts) max = Math.max(max, new Date(ts).getTime())
    }
  }
  return max
}

function groupRank(status: string): number {
  if (status === 'sent') return 0
  if (status === 'dispatched') return 1
  return 2
}

export default function OutreachCenter() {
  const [threads, setThreads] = useState<OutreachThread[] | null>(null)
  const [error, setError] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('')
  const [query, setQuery] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = () => {
      get<OutreachThread[]>('/api/outreach/all')
        .then((data) => {
          if (cancelled) return
          setThreads(data ?? [])
          setError('')
        })
        .catch((e: unknown) => {
          if (cancelled) return
          // Keep whatever we already have; only settle to empty on the first load.
          setThreads((prev) => prev ?? [])
          setError(e instanceof Error ? e.message : 'failed to load outreach')
        })
    }
    load()
    const timer = setInterval(load, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [])

  const all = threads ?? []

  const stats = useMemo(() => {
    const sent = all.filter((t) => t.status !== 'pending')
    const replies = all.reduce((n, t) => n + (t.replies?.length ?? 0), 0)
    const awaiting = all.filter((t) => t.status === 'sent').length
    const delivered = all.filter((t) => ['sent', 'replied', 'selected'].includes(t.status))
    const responded = delivered.filter((t) => (t.replies?.length ?? 0) > 0).length
    const rate = delivered.length > 0 ? Math.round((responded / delivered.length) * 100) : 0
    return { sent: sent.length, replies, awaiting, rate }
  }, [all])

  const groups = useMemo(() => {
    const q = query.trim().toLowerCase()
    const rows = all.filter((t) => {
      if (statusFilter === 'awaiting' && t.status !== 'sent') return false
      if (statusFilter === 'replied' && t.status !== 'replied') return false
      if (statusFilter === 'won' && t.status !== 'selected') return false
      if (statusFilter === 'failed' && t.status !== 'failed') return false
      if (
        q &&
        !t.vendor_name.toLowerCase().includes(q) &&
        !t.work_order_name.toLowerCase().includes(q)
      )
        return false
      return true
    })
    const map = new Map<number, OutreachThread[]>()
    for (const r of rows) {
      const arr = map.get(r.work_order_id)
      if (arr) arr.push(r)
      else map.set(r.work_order_id, [r])
    }
    return [...map.entries()].sort(([, a], [, b]) => {
      const ra = groupRank(a[0].work_order_status)
      const rb = groupRank(b[0].work_order_status)
      if (ra !== rb) return ra - rb
      return groupActivity(b) - groupActivity(a)
    })
  }, [all, statusFilter, query])

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Vendor Outreach"
        subtext="Every quote request sent to vendors, grouped by work order. Updates automatically."
      />
      <ErrorBanner message={error} />

      {threads === null ? (
        <Spinner label="Loading outreach…" />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatTile label="Messages Sent" value={stats.sent} accent="text-sky-600" />
            <StatTile label="Replies Received" value={stats.replies} accent="text-emerald-600" />
            <StatTile label="Awaiting Reply" value={stats.awaiting} accent="text-amber-600" />
            <StatTile label="Response Rate" value={`${stats.rate}%`} accent="text-indigo-600" />
          </div>

          <div className="mt-6 flex flex-wrap items-center gap-2">
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
              className={`${inputCls} w-44`}
            >
              <option value="">All statuses</option>
              <option value="awaiting">Awaiting reply</option>
              <option value="replied">Replied</option>
              <option value="won">Won</option>
              <option value="failed">Failed</option>
            </select>
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search vendor or work order…"
              className={`${inputCls} w-64`}
            />
          </div>

          {groups.length === 0 ? (
            <Card className="mt-6">
              <EmptyState
                title="No outreach threads"
                hint="Outreach threads appear here when you send quote requests from a work order."
              />
            </Card>
          ) : (
            <div className="mt-6 space-y-5">
              {groups.map(([woId, rows]) => {
                const head = rows[0]
                return (
                  <Card key={woId} className="overflow-hidden">
                    <div className="flex flex-wrap items-center gap-3 border-b border-slate-100 bg-slate-50/50 px-5 py-3">
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate text-sm font-semibold text-slate-900">
                            {head.work_order_name}
                          </span>
                          <span className="inline-flex items-center rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600">
                            {head.trade.replace(/_/g, ' ')}
                          </span>
                          <StatusBadge status={head.work_order_status} />
                        </div>
                        <div className="mt-0.5 text-xs text-slate-500">{head.property_name}</div>
                      </div>
                      <Link
                        to={`/work-orders/${woId}`}
                        className="shrink-0 text-sm font-medium text-sky-600 hover:text-sky-700"
                      >
                        View work order →
                      </Link>
                    </div>
                    <div className="divide-y divide-slate-100">
                      {rows.map((t) => {
                        const reply = latestReply(t)
                        const quote = latestQuote(t)
                        return (
                          <div key={t.id} className="px-5 py-3.5">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="text-sm font-medium text-slate-900">{t.vendor_name}</span>
                              <ChannelChip channel={t.channel} />
                              <RequestStatusBadge t={t} />
                              <span className="text-xs text-slate-400">
                                {t.sent_at ? `Sent ${fmtDate(t.sent_at)}` : `Created ${fmtDate(t.created_at)}`}
                              </span>
                              {quote?.parsed_quote_cents != null && (
                                <span className="ml-auto text-sm font-bold text-slate-900 tabular-nums">
                                  {fmtCents(quote.parsed_quote_cents)}
                                </span>
                              )}
                            </div>
                            {quote?.parsed_availability && (
                              <div className="mt-1 text-xs text-slate-500">
                                Availability: {quote.parsed_availability}
                              </div>
                            )}
                            {reply && <ReplyBlock reply={reply} />}
                          </div>
                        )
                      })}
                    </div>
                  </Card>
                )
              })}
            </div>
          )}
        </>
      )}
    </div>
  )
}
