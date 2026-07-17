import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, get, post, fmtCents, fmtDate } from '../lib/api'
import { CATEGORIES, type Activity, type OutreachReply, type OutreachRequest, type Shortlist, type WorkOrder } from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
  Modal,
  PriorityBadge,
  Spinner,
  StatusBadge,
  Toast,
  btnPrimary,
  btnSecondary,
  inputCls,
} from '../components/ui'

const patch = <T,>(path: string, body: unknown) =>
  api<T>(path, { method: 'PATCH', body: JSON.stringify(body) })

const ACTIVITY_ICONS: Record<string, string> = {
  created: '+',
  status_change: '⇄',
  note: '📝',
  outreach_sent: '✉',
  outreach_reply: '💬',
  dispatched: '✓',
  assigned: '👤',
  draft_approved: '🤖',
}

function latestReply(r: OutreachRequest): OutreachReply | null {
  if (!r.replies || r.replies.length === 0) return null
  return [...r.replies].sort(
    (a, b) => new Date(a.received_at).getTime() - new Date(b.received_at).getTime(),
  )[r.replies.length - 1]
}

export default function WorkOrderDetail() {
  const { id } = useParams<{ id: string }>()
  const woId = Number(id)

  const [wo, setWo] = useState<WorkOrder | null>(null)
  const [activity, setActivity] = useState<Activity[] | null>(null)
  const [outreach, setOutreach] = useState<OutreachRequest[] | null>(null)
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')
  const toastTimer = useRef<number | undefined>(undefined)

  // find-vendors flow
  const [showFinder, setShowFinder] = useState(false)
  const [category, setCategory] = useState('general')
  const [maxVendors, setMaxVendors] = useState('3')
  const [outreachNote, setOutreachNote] = useState('')
  const [shortlist, setShortlist] = useState<Shortlist | null>(null)
  const [previewBusy, setPreviewBusy] = useState(false)
  const [sendBusy, setSendBusy] = useState(false)

  // dispatch modal
  const [dispatchReq, setDispatchReq] = useState<OutreachRequest | null>(null)
  const [dispatchQuote, setDispatchQuote] = useState('')
  const [dispatchNote, setDispatchNote] = useState('')
  const [notifyVendor, setNotifyVendor] = useState(true)
  const [dispatchBusy, setDispatchBusy] = useState(false)

  // activity note composer + misc
  const [noteBody, setNoteBody] = useState('')
  const [noteBusy, setNoteBusy] = useState(false)
  const [statusBusy, setStatusBusy] = useState(false)
  const [expandedReplies, setExpandedReplies] = useState<Record<number, boolean>>({})

  const showToast = useCallback((msg: string) => {
    setToast(msg)
    window.clearTimeout(toastTimer.current)
    toastTimer.current = window.setTimeout(() => setToast(''), 3500)
  }, [])

  const loadWo = useCallback(async () => {
    const w = await get<WorkOrder>(`/api/work-orders/${woId}`)
    setWo(w)
    if (w.category) setCategory(w.category)
  }, [woId])

  const loadActivity = useCallback(async () => {
    setActivity(await get<Activity[]>(`/api/activity/work_order/${woId}`))
  }, [woId])

  const loadOutreach = useCallback(async () => {
    setOutreach((await get<OutreachRequest[]>(`/api/outreach/work-orders/${woId}`)) ?? [])
  }, [woId])

  useEffect(() => {
    if (!woId) return
    setError('')
    Promise.all([loadWo(), loadActivity(), loadOutreach()]).catch((e: unknown) =>
      setError(e instanceof Error ? e.message : 'failed to load work order'),
    )
  }, [woId, loadWo, loadActivity, loadOutreach])

  // Poll outreach every 5s while any request is still awaiting a reply.
  const awaitingReplies = (outreach ?? []).some((r) => r.status === 'sent' || r.status === 'pending')
  useEffect(() => {
    if (!awaitingReplies) return
    const t = window.setInterval(() => {
      loadOutreach().catch(() => {})
      loadActivity().catch(() => {})
    }, 5000)
    return () => window.clearInterval(t)
  }, [awaitingReplies, loadOutreach, loadActivity])

  const previewShortlist = async () => {
    setPreviewBusy(true)
    setError('')
    try {
      const max = Math.max(1, Number(maxVendors) || 3)
      setShortlist(
        await get<Shortlist>(
          `/api/outreach/work-orders/${woId}/shortlist?category=${encodeURIComponent(category)}&max=${max}`,
        ),
      )
    } catch (e) {
      setError(e instanceof Error ? e.message : 'shortlist lookup failed')
    } finally {
      setPreviewBusy(false)
    }
  }

  const sendOutreach = async () => {
    setSendBusy(true)
    setError('')
    try {
      const body: Record<string, unknown> = {
        category,
        max_vendors: Math.max(1, Number(maxVendors) || 3),
      }
      if (shortlist) body.vendor_ids = shortlist.vendors.map((v) => v.id)
      if (outreachNote.trim()) body.note = outreachNote.trim()
      await post(`/api/outreach/work-orders/${woId}/trigger`, body)
      showToast('Outreach sent — watching for vendor replies')
      setShortlist(null)
      setShowFinder(false)
      await Promise.all([loadOutreach(), loadWo(), loadActivity()])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'outreach failed')
    } finally {
      setSendBusy(false)
    }
  }

  const openDispatch = (r: OutreachRequest) => {
    const reply = latestReply(r)
    setDispatchReq(r)
    setDispatchQuote(
      reply?.parsed_quote_cents != null ? (reply.parsed_quote_cents / 100).toFixed(2) : '',
    )
    setDispatchNote('')
    setNotifyVendor(true)
  }

  const confirmDispatch = async () => {
    if (!dispatchReq) return
    setDispatchBusy(true)
    setError('')
    try {
      const reply = latestReply(dispatchReq)
      const body: Record<string, unknown> = {
        vendor_id: dispatchReq.vendor_id,
        notify_vendor: notifyVendor,
      }
      const dollars = parseFloat(dispatchQuote)
      if (!Number.isNaN(dollars)) body.quote_cents = Math.round(dollars * 100)
      if (reply) body.reply_id = reply.id
      if (dispatchNote.trim()) body.note = dispatchNote.trim()
      await post(`/api/outreach/work-orders/${woId}/dispatch`, body)
      setDispatchReq(null)
      showToast(`Dispatched to ${dispatchReq.vendor_name}`)
      await Promise.all([loadWo(), loadOutreach(), loadActivity()])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'dispatch failed')
    } finally {
      setDispatchBusy(false)
    }
  }

  const setStatus = async (status: string) => {
    setStatusBusy(true)
    setError('')
    try {
      if (status === 'completed') await post(`/api/work-orders/${woId}/complete`)
      else await patch(`/api/work-orders/${woId}`, { status })
      showToast(`Status updated`)
      await Promise.all([loadWo(), loadActivity()])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'status update failed')
    } finally {
      setStatusBusy(false)
    }
  }

  const addNote = async () => {
    if (!noteBody.trim()) return
    setNoteBusy(true)
    try {
      await post(`/api/activity/work_order/${woId}/notes`, { body: noteBody.trim() })
      setNoteBody('')
      await loadActivity()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to add note')
    } finally {
      setNoteBusy(false)
    }
  }

  if (!wo) {
    return (
      <div className="p-8 bg-slate-50 min-h-screen">
        <ErrorBanner message={error} />
        {!error && <Spinner label="Loading work order…" />}
      </div>
    )
  }

  const hasOutreach = (outreach ?? []).length > 0

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <Toast message={toast} />
      <div className="mb-2 text-sm">
        <Link to="/" className="text-sky-600 hover:underline">
          ← Dashboard
        </Link>
      </div>
      <ErrorBanner message={error} />

      {/* Header */}
      <Card className="p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight text-slate-900">{wo.name}</h1>
              <StatusBadge status={wo.status} />
              <PriorityBadge priority={wo.priority} />
            </div>
            <div className="mt-1 text-sm text-slate-500">
              {wo.property_name ?? `Property #${wo.property_id}`}
              {wo.category ? ` · ${wo.category.replace(/_/g, ' ')}` : ''}
              {wo.due_date ? ` · due ${fmtDate(wo.due_date)}` : ''}
            </div>
            {wo.work_description && <p className="mt-3 max-w-3xl text-sm text-slate-600">{wo.work_description}</p>}
            {(wo.media_urls?.length ?? 0) > 0 && (
              <div className="mt-3 flex flex-wrap gap-2">
                {(wo.media_urls ?? []).map((u, i) => (
                  <a key={i} href={u} target="_blank" rel="noreferrer">
                    <img
                      src={u}
                      alt={`photo ${i + 1}`}
                      className="h-20 w-20 rounded-lg border border-slate-200 object-cover hover:opacity-90"
                    />
                  </a>
                ))}
              </div>
            )}
          </div>
          <div className="text-right">
            {wo.quote_amount_cents != null && (
              <div>
                <div className="text-xs font-medium uppercase tracking-wide text-slate-500">Quote</div>
                <div className="text-2xl font-semibold text-slate-900 tabular-nums">
                  {fmtCents(wo.quote_amount_cents)}
                </div>
              </div>
            )}
            <div className="mt-2 text-xs text-slate-400">Created {fmtDate(wo.created_at)}</div>
            {wo.dispatched_at && <div className="text-xs text-slate-400">Dispatched {fmtDate(wo.dispatched_at)}</div>}
          </div>
        </div>

        {/* Status actions */}
        <div className="mt-5 flex flex-wrap gap-2 border-t border-slate-100 pt-4">
          <button
            className={btnSecondary}
            disabled={statusBusy || wo.status === 'scheduled'}
            onClick={() => void setStatus('scheduled')}
          >
            Mark Scheduled
          </button>
          <button
            className={btnSecondary}
            disabled={statusBusy || wo.status === 'in_progress'}
            onClick={() => void setStatus('in_progress')}
          >
            Start Work
          </button>
          <button
            className={btnPrimary}
            disabled={statusBusy || wo.status === 'completed' || wo.status === 'closed'}
            onClick={() => void setStatus('completed')}
          >
            Mark Complete
          </button>
        </div>
      </Card>

      <div className="mt-6 grid gap-6 xl:grid-cols-3">
        {/* Vendor Outreach panel */}
        <Card className="xl:col-span-2 p-6">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-base font-semibold text-slate-900">Vendor Outreach</h2>
              <p className="text-sm text-slate-500">
                Source quotes by texting and emailing vendors, then dispatch the winner.
              </p>
            </div>
            {hasOutreach && !showFinder && (
              <button className={btnSecondary} onClick={() => setShowFinder(true)}>
                + More vendors
              </button>
            )}
          </div>

          {outreach === null ? (
            <Spinner />
          ) : (
            <>
              {(!hasOutreach || showFinder) && (
                <div className="mt-4 rounded-lg border border-dashed border-slate-300 bg-slate-50/60 p-4">
                  <div className="grid gap-3 sm:grid-cols-3">
                    <div>
                      <label className="block text-xs font-medium text-slate-600 mb-1">Category</label>
                      <select value={category} onChange={(e) => setCategory(e.target.value)} className={inputCls}>
                        {CATEGORIES.map((c) => (
                          <option key={c} value={c}>
                            {c.replace(/_/g, ' ')}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-slate-600 mb-1">Max vendors</label>
                      <input
                        type="number"
                        min={1}
                        max={10}
                        value={maxVendors}
                        onChange={(e) => setMaxVendors(e.target.value)}
                        className={inputCls}
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-slate-600 mb-1">Note (optional)</label>
                      <input
                        value={outreachNote}
                        onChange={(e) => setOutreachNote(e.target.value)}
                        placeholder="e.g. tenant home after 3pm"
                        className={inputCls}
                      />
                    </div>
                  </div>

                  {shortlist && (
                    <div className="mt-4">
                      <div className="text-xs font-medium text-slate-500">
                        {shortlist.source === 'preferred_list'
                          ? 'From preferred list for this property'
                          : 'From local directory lookup'}
                      </div>
                      {shortlist.vendors.length === 0 ? (
                        <div className="mt-2 text-sm text-slate-500">No vendors found for this category.</div>
                      ) : (
                        <div className="mt-2 grid gap-2 sm:grid-cols-3">
                          {shortlist.vendors.map((v) => (
                            <div key={v.id} className="rounded-lg border border-slate-200 bg-white p-3">
                              <div className="text-sm font-semibold text-slate-900">{v.name}</div>
                              <div className="text-xs text-slate-500 capitalize">{v.category.replace(/_/g, ' ')}</div>
                              <div className="mt-1 text-xs text-slate-500">
                                {v.phone ?? v.primary_email ?? '—'}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}

                  <div className="mt-4 flex gap-2">
                    <button className={btnSecondary} disabled={previewBusy} onClick={() => void previewShortlist()}>
                      {previewBusy ? 'Looking up…' : 'Preview shortlist'}
                    </button>
                    <button
                      className={btnPrimary}
                      disabled={sendBusy || (shortlist !== null && shortlist.vendors.length === 0)}
                      onClick={() => void sendOutreach()}
                    >
                      {sendBusy ? 'Sending…' : 'Send outreach'}
                    </button>
                    {hasOutreach && (
                      <button className={btnSecondary} onClick={() => setShowFinder(false)}>
                        Cancel
                      </button>
                    )}
                  </div>
                </div>
              )}

              {hasOutreach && (
                <div className="mt-4 overflow-x-auto">
                  {awaitingReplies && (
                    <div className="mb-2 flex items-center gap-2 text-xs text-sky-600">
                      <span className="relative flex h-2 w-2">
                        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-sky-400 opacity-75" />
                        <span className="relative inline-flex h-2 w-2 rounded-full bg-sky-500" />
                      </span>
                      Live — checking for vendor replies every 5 seconds
                    </div>
                  )}
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-slate-200 text-left text-xs font-medium uppercase tracking-wide text-slate-500">
                        <th className="py-2.5 pr-4">Vendor</th>
                        <th className="py-2.5 pr-4">Channel</th>
                        <th className="py-2.5 pr-4">Status</th>
                        <th className="py-2.5 pr-4">Sent</th>
                        <th className="py-2.5 pr-4">Latest reply</th>
                        <th className="py-2.5 pr-4">Quote</th>
                        <th className="py-2.5 pr-4">Availability</th>
                        <th className="py-2.5" />
                      </tr>
                    </thead>
                    <tbody>
                      {(outreach ?? []).map((r) => {
                        const reply = latestReply(r)
                        const expanded = expandedReplies[r.id]
                        const replyText = reply?.body ?? ''
                        return (
                          <tr
                            key={r.id}
                            className={`border-b border-slate-100 last:border-0 align-top ${
                              r.status === 'selected' ? 'bg-sky-50/60' : ''
                            }`}
                          >
                            <td className="py-3 pr-4">
                              <div className="font-medium text-slate-900">{r.vendor_name}</div>
                              <div className="text-xs text-slate-400">{r.to_address}</div>
                            </td>
                            <td className="py-3 pr-4 uppercase text-xs text-slate-500 font-medium">{r.channel}</td>
                            <td className="py-3 pr-4">
                              <span title={r.status === 'failed' ? (r.error ?? undefined) : undefined}>
                                <StatusBadge status={r.status} label={r.status === 'sent' ? 'Sent' : undefined} />
                              </span>
                            </td>
                            <td className="py-3 pr-4 whitespace-nowrap text-slate-500">{fmtDate(r.sent_at)}</td>
                            <td className="py-3 pr-4 max-w-56">
                              {replyText ? (
                                <div className="text-slate-600">
                                  <span>{expanded || replyText.length <= 70 ? replyText : `${replyText.slice(0, 70)}…`}</span>
                                  {replyText.length > 70 && (
                                    <button
                                      className="ml-1 text-xs text-sky-600 hover:underline"
                                      onClick={() =>
                                        setExpandedReplies((m) => ({ ...m, [r.id]: !m[r.id] }))
                                      }
                                    >
                                      {expanded ? 'less' : 'more'}
                                    </button>
                                  )}
                                </div>
                              ) : (
                                <span className="text-slate-300">—</span>
                              )}
                            </td>
                            <td className="py-3 pr-4 font-semibold tabular-nums text-slate-900">
                              {reply?.parsed_quote_cents != null ? fmtCents(reply.parsed_quote_cents) : '—'}
                            </td>
                            <td className="py-3 pr-4 text-slate-600">{reply?.parsed_availability ?? '—'}</td>
                            <td className="py-3 text-right">
                              {reply && r.status !== 'selected' && (
                                <button
                                  className="rounded-md bg-sky-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-sky-700"
                                  onClick={() => openDispatch(r)}
                                >
                                  Dispatch
                                </button>
                              )}
                              {r.status === 'selected' && (
                                <span className="text-xs font-medium text-sky-700">Winner</span>
                              )}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </Card>

        {/* Activity feed */}
        <Card className="p-6">
          <h2 className="text-base font-semibold text-slate-900">Activity</h2>
          {activity === null ? (
            <Spinner />
          ) : activity.length === 0 ? (
            <EmptyState title="No activity yet" />
          ) : (
            <ol className="mt-4 space-y-0">
              {activity.map((a, i) => (
                <li key={a.id} className="relative flex gap-3 pb-5">
                  {i < activity.length - 1 && (
                    <span className="absolute left-[13px] top-7 bottom-0 w-px bg-slate-200" aria-hidden />
                  )}
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-slate-200 bg-slate-50 text-xs">
                    {ACTIVITY_ICONS[a.kind] ?? '•'}
                  </span>
                  <div className="min-w-0">
                    <div className="text-xs text-slate-400">
                      <span className="font-medium text-slate-600">
                        {a.actor_name || (a.actor_type === 'ai' ? 'AI assistant' : a.actor_type)}
                      </span>{' '}
                      · {fmtDate(a.created_at)}
                    </div>
                    <div className="mt-0.5 text-sm text-slate-700 break-words">{a.body}</div>
                  </div>
                </li>
              ))}
            </ol>
          )}
          <div className="mt-2 border-t border-slate-100 pt-4">
            <textarea
              value={noteBody}
              onChange={(e) => setNoteBody(e.target.value)}
              rows={2}
              placeholder="Add an internal note…"
              className={inputCls}
            />
            <button
              className={`${btnPrimary} mt-2`}
              disabled={noteBusy || !noteBody.trim()}
              onClick={() => void addNote()}
            >
              {noteBusy ? 'Posting…' : 'Add note'}
            </button>
          </div>
        </Card>
      </div>

      {/* Dispatch confirm modal */}
      {dispatchReq && (
        <Modal
          title={`Dispatch ${dispatchReq.vendor_name}`}
          onClose={() => setDispatchReq(null)}
          footer={
            <>
              <button className={btnSecondary} onClick={() => setDispatchReq(null)}>
                Cancel
              </button>
              <button className={btnPrimary} disabled={dispatchBusy} onClick={() => void confirmDispatch()}>
                {dispatchBusy ? 'Dispatching…' : 'Confirm dispatch'}
              </button>
            </>
          }
        >
          <p className="text-sm text-slate-600">
            This vendor will be selected for the work order and the ticket will move to{' '}
            <span className="font-medium">Dispatched</span>.
          </p>
          <div className="mt-4">
            <label className="block text-xs font-medium text-slate-600 mb-1">Quote (USD)</label>
            <input
              value={dispatchQuote}
              onChange={(e) => setDispatchQuote(e.target.value)}
              placeholder="e.g. 450.00"
              className={inputCls}
            />
          </div>
          <div className="mt-3">
            <label className="block text-xs font-medium text-slate-600 mb-1">Note (optional)</label>
            <textarea
              value={dispatchNote}
              onChange={(e) => setDispatchNote(e.target.value)}
              rows={2}
              className={inputCls}
            />
          </div>
          <label className="mt-3 flex items-center gap-2 text-sm text-slate-700">
            <input
              type="checkbox"
              checked={notifyVendor}
              onChange={(e) => setNotifyVendor(e.target.checked)}
              className="h-4 w-4 rounded border-slate-300 text-sky-600 focus:ring-sky-500"
            />
            Notify vendor they got the job
          </label>
        </Modal>
      )}
    </div>
  )
}
