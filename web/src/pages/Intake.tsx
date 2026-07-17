import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { get, post, fmtDate } from '../lib/api'
import {
  CATEGORIES,
  PRIORITIES,
  type FTO,
  type Property,
  type SmsConversation,
  type SmsMessage,
  type Task,
  type WorkOrder,
} from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
  PageHeader,
  Spinner,
  btnPrimary,
  btnSecondary,
  inputCls,
} from '../components/ui'

function convLabel(c: SmsConversation): string {
  return c.contact_name || c.phone_number || c.from_number || `Conversation #${c.id}`
}

export default function Intake() {
  const navigate = useNavigate()
  const [properties, setProperties] = useState<Property[]>([])
  const [error, setError] = useState('')

  // manual form
  const [propertyId, setPropertyId] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('general')
  const [priority, setPriority] = useState('medium')
  const [alsoWO, setAlsoWO] = useState(true)
  const [alsoFTO, setAlsoFTO] = useState(false)
  const [submitBusy, setSubmitBusy] = useState(false)
  const [created, setCreated] = useState<{ task: Task; fto?: FTO } | null>(null)

  // sms demo panel
  const [conversations, setConversations] = useState<SmsConversation[] | null>(null)
  const [activeConv, setActiveConv] = useState<SmsConversation | null>(null)
  const [messages, setMessages] = useState<SmsMessage[] | null>(null)
  const [smsError, setSmsError] = useState('')

  useEffect(() => {
    get<Property[]>('/api/properties')
      .then((p) => setProperties(p ?? []))
      .catch((e: unknown) => setError(e instanceof Error ? e.message : 'failed to load properties'))
    get<SmsConversation[]>('/api/sms-admin/conversations')
      .then((c) => setConversations(c ?? []))
      .catch((e: unknown) => {
        setSmsError(e instanceof Error ? e.message : 'failed to load conversations')
        setConversations([])
      })
  }, [])

  const openConversation = async (c: SmsConversation) => {
    setActiveConv(c)
    setMessages(null)
    setSmsError('')
    try {
      setMessages((await get<SmsMessage[]>(`/api/sms-admin/conversations/${c.id}/messages`)) ?? [])
    } catch (e) {
      setSmsError(e instanceof Error ? e.message : 'failed to load messages')
      setMessages([])
    }
  }

  const submit = async () => {
    if (!propertyId || !name.trim()) {
      setError('Property and name are required')
      return
    }
    setSubmitBusy(true)
    setError('')
    setCreated(null)
    try {
      const task = await post<Task>('/api/tasks', {
        property_id: Number(propertyId),
        name: name.trim(),
        description: description.trim(),
        priority,
        category,
        status: 'open',
      })

      let wo: WorkOrder | undefined
      if (alsoWO) {
        wo = await post<WorkOrder>('/api/work-orders', {
          property_id: Number(propertyId),
          name: name.trim(),
          description: description.trim(),
          priority,
          status: 'new',
          task_id: task.id,
        })
      }

      let fto: FTO | undefined
      if (alsoFTO) {
        fto = await post<FTO>('/api/ftos', {
          task_id: task.id,
          name: name.trim(),
          description: description.trim(),
          property_id: Number(propertyId),
          priority,
        })
      }

      if (wo) {
        navigate(`/work-orders/${wo.id}`)
        return
      }
      setCreated({ task, fto })
      setName('')
      setDescription('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to create request')
    } finally {
      setSubmitBusy(false)
    }
  }

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="New Request"
        subtext="Log a maintenance request by hand, or let SMS intake draft it for you."
      />

      <div className="grid gap-6 xl:grid-cols-2">
        {/* Manual ticket */}
        <Card className="p-6">
          <h2 className="text-base font-semibold text-slate-900">Manual ticket</h2>
          <p className="mt-0.5 text-sm text-slate-500">Creates a task, plus optional downstream tickets.</p>
          <ErrorBanner message={error} />

          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <label className="block text-xs font-medium text-slate-600 mb-1">Property</label>
              <select value={propertyId} onChange={(e) => setPropertyId(e.target.value)} className={inputCls}>
                <option value="">Select a property…</option>
                {properties.map((p) => (
                  <option key={p.id} value={String(p.id)}>
                    {p.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="sm:col-span-2">
              <label className="block text-xs font-medium text-slate-600 mb-1">Name</label>
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Leaking kitchen faucet in unit 4B"
                className={inputCls}
              />
            </div>
            <div className="sm:col-span-2">
              <label className="block text-xs font-medium text-slate-600 mb-1">Description</label>
              <textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={3}
                placeholder="What's wrong, where, and any access notes…"
                className={inputCls}
              />
            </div>
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
              <label className="block text-xs font-medium text-slate-600 mb-1">Priority</label>
              <select value={priority} onChange={(e) => setPriority(e.target.value)} className={inputCls}>
                {PRIORITIES.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="mt-4 space-y-2 rounded-lg bg-slate-50 p-4">
            <label className="flex items-center gap-2 text-sm text-slate-700">
              <input
                type="checkbox"
                checked={alsoWO}
                onChange={(e) => setAlsoWO(e.target.checked)}
                className="h-4 w-4 rounded border-slate-300 text-sky-600 focus:ring-sky-500"
              />
              Also create a <span className="font-medium">Work Order</span> (external vendor)
            </label>
            <label className="flex items-center gap-2 text-sm text-slate-700">
              <input
                type="checkbox"
                checked={alsoFTO}
                onChange={(e) => setAlsoFTO(e.target.checked)}
                className="h-4 w-4 rounded border-slate-300 text-sky-600 focus:ring-sky-500"
              />
              Also create a <span className="font-medium">Field Team Order</span> (internal crew)
            </label>
          </div>

          <button className={`${btnPrimary} mt-4`} disabled={submitBusy} onClick={() => void submit()}>
            {submitBusy ? 'Creating…' : 'Create request'}
          </button>

          {created && (
            <div className="mt-4 rounded-md border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">
              Request created.{' '}
              <Link to={`/tickets/task/${created.task.id}`} className="font-medium underline">
                View task
              </Link>
              {created.fto && (
                <>
                  {' · '}
                  <Link to={`/tickets/fto/${created.fto.id}`} className="font-medium underline">
                    View field team order
                  </Link>
                </>
              )}
            </div>
          )}
        </Card>

        {/* SMS intake demo */}
        <Card className="p-6">
          <h2 className="text-base font-semibold text-slate-900">SMS intake</h2>
          <p className="mt-0.5 text-sm text-slate-500">
            Tenants and owners can simply text your org number. The AI reads the thread, drafts a ticket with a
            work order or field team order attached, and drops it in the{' '}
            <Link to="/drafts" className="text-sky-600 hover:underline">
              Review Queue
            </Link>{' '}
            for one-click approval.
          </p>
          <ErrorBanner message={smsError} />

          {conversations === null ? (
            <Spinner />
          ) : conversations.length === 0 ? (
            <EmptyState title="No SMS conversations yet" hint="Text the org number to see the thread appear here." />
          ) : (
            <div className="mt-4 grid gap-4 sm:grid-cols-5">
              <div className="sm:col-span-2 space-y-1 max-h-96 overflow-y-auto pr-1">
                {conversations.map((c) => (
                  <button
                    key={c.id}
                    onClick={() => void openConversation(c)}
                    className={`w-full rounded-lg border px-3 py-2 text-left transition-colors ${
                      activeConv?.id === c.id
                        ? 'border-sky-300 bg-sky-50'
                        : 'border-slate-200 bg-white hover:bg-slate-50'
                    }`}
                  >
                    <div className="text-sm font-medium text-slate-900">{convLabel(c)}</div>
                    {c.last_message && (
                      <div className="mt-0.5 truncate text-xs text-slate-500">{c.last_message}</div>
                    )}
                    <div className="mt-0.5 text-xs text-slate-400">
                      {fmtDate(c.last_message_at ?? c.updated_at ?? c.created_at)}
                    </div>
                  </button>
                ))}
              </div>
              <div className="sm:col-span-3 rounded-lg border border-slate-200 bg-slate-50/70 p-3 max-h-96 overflow-y-auto">
                {!activeConv ? (
                  <EmptyState title="Pick a conversation" hint="Messages will show here." />
                ) : messages === null ? (
                  <Spinner label="Loading messages…" />
                ) : messages.length === 0 ? (
                  <EmptyState title="No messages" />
                ) : (
                  <div className="space-y-2">
                    {messages.map((m) => {
                      const outbound = (m.direction ?? '').toLowerCase().startsWith('out')
                      return (
                        <div key={m.id} className={`flex ${outbound ? 'justify-end' : 'justify-start'}`}>
                          <div
                            className={`max-w-[80%] rounded-2xl px-3 py-2 text-sm ${
                              outbound
                                ? 'bg-sky-600 text-white rounded-br-sm'
                                : 'bg-white border border-slate-200 text-slate-800 rounded-bl-sm'
                            }`}
                          >
                            <div className="whitespace-pre-wrap break-words">{m.body ?? ''}</div>
                            <div className={`mt-1 text-[10px] ${outbound ? 'text-sky-100' : 'text-slate-400'}`}>
                              {fmtDate(m.created_at)}
                            </div>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            </div>
          )}

          <div className="mt-4 flex gap-2">
            <Link to="/drafts" className={btnSecondary}>
              Open Review Queue →
            </Link>
          </div>
        </Card>
      </div>
    </div>
  )
}
