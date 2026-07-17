import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, get, post, fmtDate } from '../lib/api'
import type { Activity, FTO, Task } from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
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

type Ticket = {
  id: number
  name: string
  description?: string | null
  property_name?: string | null
  property_id?: number | null
  status: string
  priority: string
  created_at: string
  field_team_member_ids?: number[]
}

export default function TicketDetail() {
  const { type, id } = useParams<{ type: string; id: string }>()
  const ticketId = Number(id)
  const isFto = type === 'fto'
  const activityType = isFto ? 'fto' : 'task'

  const [ticket, setTicket] = useState<Ticket | null>(null)
  const [activity, setActivity] = useState<Activity[] | null>(null)
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')
  const [noteBody, setNoteBody] = useState('')
  const [noteBusy, setNoteBusy] = useState(false)
  const [statusBusy, setStatusBusy] = useState(false)
  const [assignInput, setAssignInput] = useState('')
  const [assignBusy, setAssignBusy] = useState(false)

  const showToast = (msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast(''), 3000)
  }

  // No single-item GET exists for tasks/FTOs, so fetch the list and find by id.
  const loadTicket = useCallback(async () => {
    if (isFto) {
      const list = await get<FTO[]>('/api/ftos?limit=200')
      const f = (list ?? []).find((x) => x.id === ticketId)
      if (!f) throw new Error('field team order not found')
      setTicket({ ...f, description: f.description ?? null })
      setAssignInput((f.field_team_member_ids ?? []).join(', '))
    } else {
      const list = await get<Task[]>('/api/tasks?limit=200')
      const t = (list ?? []).find((x) => x.id === ticketId)
      if (!t) throw new Error('task not found')
      setTicket(t)
    }
  }, [isFto, ticketId])

  const loadActivity = useCallback(async () => {
    setActivity(await get<Activity[]>(`/api/activity/${activityType}/${ticketId}`))
  }, [activityType, ticketId])

  useEffect(() => {
    setError('')
    Promise.all([loadTicket(), loadActivity()]).catch((e: unknown) =>
      setError(e instanceof Error ? e.message : 'failed to load ticket'),
    )
  }, [loadTicket, loadActivity])

  const setStatus = async (status: string) => {
    setStatusBusy(true)
    setError('')
    try {
      if (isFto) {
        if (status === 'completed') await post(`/api/ftos/${ticketId}/complete`)
        else await patch(`/api/ftos/${ticketId}`, { status })
      } else {
        await patch(`/api/tasks/${ticketId}`, { status })
      }
      showToast('Status updated')
      await Promise.all([loadTicket(), loadActivity()])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'status update failed')
    } finally {
      setStatusBusy(false)
    }
  }

  const assign = async () => {
    setAssignBusy(true)
    setError('')
    try {
      const ids = assignInput
        .split(/[,\s]+/)
        .map((s) => Number(s))
        .filter((n) => Number.isInteger(n) && n > 0)
      await patch(`/api/ftos/${ticketId}`, { field_team_member_ids: ids })
      showToast('Team members assigned')
      await Promise.all([loadTicket(), loadActivity()])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'assignment failed')
    } finally {
      setAssignBusy(false)
    }
  }

  const addNote = async () => {
    if (!noteBody.trim()) return
    setNoteBusy(true)
    try {
      await post(`/api/activity/${activityType}/${ticketId}/notes`, { body: noteBody.trim() })
      setNoteBody('')
      await loadActivity()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to add note')
    } finally {
      setNoteBusy(false)
    }
  }

  if (!ticket) {
    return (
      <div className="p-8 bg-slate-50 min-h-screen">
        <ErrorBanner message={error} />
        {!error && <Spinner label="Loading ticket…" />}
      </div>
    )
  }

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <Toast message={toast} />
      <div className="mb-2 text-sm">
        <Link to="/" className="text-sky-600 hover:underline">
          ← Dashboard
        </Link>
      </div>
      <ErrorBanner message={error} />

      <Card className="p-6">
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-medium text-slate-600">
            {isFto ? 'Field Team Order' : 'Task'}
          </span>
          <h1 className="text-2xl font-semibold tracking-tight text-slate-900">{ticket.name}</h1>
          <StatusBadge status={ticket.status} />
          <PriorityBadge priority={ticket.priority} />
        </div>
        <div className="mt-1 text-sm text-slate-500">
          {ticket.property_name ?? (ticket.property_id ? `Property #${ticket.property_id}` : 'No property')}
          {' · '}created {fmtDate(ticket.created_at)}
        </div>
        {ticket.description && <p className="mt-3 max-w-3xl text-sm text-slate-600">{ticket.description}</p>}

        <div className="mt-5 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-4">
          {isFto && (
            <button
              className={btnSecondary}
              disabled={statusBusy || ticket.status === 'dispatched'}
              onClick={() => void setStatus('dispatched')}
            >
              Dispatch
            </button>
          )}
          <button
            className={btnSecondary}
            disabled={statusBusy || ticket.status === 'scheduled'}
            onClick={() => void setStatus('scheduled')}
          >
            Mark Scheduled
          </button>
          <button
            className={btnSecondary}
            disabled={statusBusy || ticket.status === 'in_progress'}
            onClick={() => void setStatus('in_progress')}
          >
            Start Work
          </button>
          <button
            className={btnPrimary}
            disabled={statusBusy || ticket.status === 'completed' || ticket.status === 'closed'}
            onClick={() => void setStatus('completed')}
          >
            Mark Complete
          </button>
        </div>

        {isFto && (
          <div className="mt-4 rounded-lg bg-slate-50 p-4">
            <label className="block text-xs font-medium text-slate-600 mb-1">
              Assigned team members (comma-separated user IDs)
            </label>
            <div className="flex gap-2">
              <input
                value={assignInput}
                onChange={(e) => setAssignInput(e.target.value)}
                placeholder="e.g. 1, 4"
                className={`${inputCls} max-w-xs`}
              />
              <button className={btnSecondary} disabled={assignBusy} onClick={() => void assign()}>
                {assignBusy ? 'Saving…' : 'Assign'}
              </button>
            </div>
          </div>
        )}
      </Card>

      <Card className="mt-6 p-6 max-w-3xl">
        <h2 className="text-base font-semibold text-slate-900">Activity</h2>
        {activity === null ? (
          <Spinner />
        ) : activity.length === 0 ? (
          <EmptyState title="No activity yet" />
        ) : (
          <ol className="mt-4">
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
          <button className={`${btnPrimary} mt-2`} disabled={noteBusy || !noteBody.trim()} onClick={() => void addNote()}>
            {noteBusy ? 'Posting…' : 'Add note'}
          </button>
        </div>
      </Card>
    </div>
  )
}
