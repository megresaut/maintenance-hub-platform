import { useCallback, useEffect, useState } from 'react'
import { get, post } from '../lib/api'
import { fmtDate } from '../lib/api'
import type { AIDraft, Property } from '../lib/types'
import {
  Card,
  ConfidencePill,
  EmptyState,
  ErrorBanner,
  PageHeader,
  PriorityBadge,
  Spinner,
  StatusBadge,
  btnPrimary,
  btnSecondary,
  inputCls,
} from '../components/ui'

type DraftTab = 'pending' | 'approved' | 'rejected'

const SOURCE_STYLES: Record<string, string> = {
  sms: 'bg-emerald-50 text-emerald-700',
  calendar: 'bg-violet-50 text-violet-700',
  manual: 'bg-slate-100 text-slate-600',
}

const SOURCE_LABELS: Record<string, string> = { sms: 'SMS', calendar: 'Calendar', manual: 'Manual' }

function str(v: unknown): string {
  return typeof v === 'string' ? v : v === null || v === undefined ? '' : String(v)
}

function childLabel(entity: string): string {
  return entity === 'work_order' ? '+ Work Order' : entity === 'fto' ? '+ Field Team Order' : '+ Task'
}

export default function Drafts() {
  const [tab, setTab] = useState<DraftTab>('pending')
  const [drafts, setDrafts] = useState<AIDraft[] | null>(null)
  const [properties, setProperties] = useState<Property[]>([])
  const [error, setError] = useState('')
  const [busyId, setBusyId] = useState<number | null>(null)
  // draft id -> needs property picker; and the chosen property per draft
  const [needsProperty, setNeedsProperty] = useState<Record<number, string>>({})
  const [propertyChoice, setPropertyChoice] = useState<Record<number, string>>({})

  const load = useCallback(async (which: DraftTab) => {
    setDrafts(null)
    setError('')
    try {
      const list = await get<AIDraft[]>(`/api/drafts?status=${which}`)
      setDrafts((list ?? []).filter((d) => !d.parent_draft_id))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load drafts')
      setDrafts([])
    }
  }, [])

  useEffect(() => {
    void load(tab)
  }, [tab, load])

  useEffect(() => {
    get<Property[]>('/api/properties')
      .then((p) => setProperties(p ?? []))
      .catch(() => {})
  }, [])

  const propertyName = (id: unknown): string => {
    const n = typeof id === 'number' ? id : Number(id)
    if (!n || Number.isNaN(n)) return ''
    return properties.find((p) => p.id === n)?.name ?? `Property #${n}`
  }

  const approve = async (draft: AIDraft) => {
    setBusyId(draft.id)
    setError('')
    try {
      const overrideId = propertyChoice[draft.id]
      const body: { cascade: boolean; field_overrides?: Record<string, unknown> } = { cascade: true }
      if (overrideId) body.field_overrides = { property_id: Number(overrideId) }
      await post<AIDraft>(`/api/drafts/${draft.id}/approve`, body)
      setNeedsProperty((m) => {
        const next = { ...m }
        delete next[draft.id]
        return next
      })
      await load(tab)
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'approve failed'
      if (/propert/i.test(msg)) {
        setNeedsProperty((m) => ({ ...m, [draft.id]: msg }))
      } else {
        setError(msg)
      }
    } finally {
      setBusyId(null)
    }
  }

  const reject = async (draft: AIDraft) => {
    setBusyId(draft.id)
    setError('')
    try {
      await post(`/api/drafts/${draft.id}/reject`, {})
      await load(tab)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'reject failed')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Review Queue"
        subtext="AI-drafted tickets from SMS and calendar intake, ready for a human to approve."
      />
      <ErrorBanner message={error} />

      <div className="mb-4 flex gap-1 rounded-lg border border-slate-200 bg-white p-1 w-fit shadow-sm">
        {(['pending', 'approved', 'rejected'] as DraftTab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-md px-4 py-1.5 text-sm font-medium capitalize transition-colors ${
              tab === t ? 'bg-sky-600 text-white' : 'text-slate-600 hover:bg-slate-100'
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {drafts === null ? (
        <Spinner />
      ) : drafts.length === 0 ? (
        <Card>
          <EmptyState
            title={`No ${tab} drafts`}
            hint={tab === 'pending' ? 'Text the org number or add a calendar event to see AI drafts appear here.' : undefined}
          />
        </Card>
      ) : (
        <div className="space-y-4">
          {drafts.map((d) => {
            const data = d.extracted_data ?? {}
            const name = str(data['name']) || '(untitled draft)'
            const description = str(data['description'])
            const priority = str(data['priority'])
            const propName = propertyName(data['property_id'])
            return (
              <Card key={d.id} className="p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                      SOURCE_STYLES[d.source] ?? 'bg-slate-100 text-slate-600'
                    }`}
                  >
                    {SOURCE_LABELS[d.source] ?? d.source}
                  </span>
                  <ConfidencePill value={d.ai_confidence} />
                  {tab !== 'pending' && <StatusBadge status={d.status} label={d.status} />}
                  <span className="ml-auto text-xs text-slate-400">{fmtDate(d.created_at)}</span>
                </div>

                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <h3 className="text-base font-semibold text-slate-900">{name}</h3>
                  {priority && <PriorityBadge priority={priority} />}
                </div>
                {propName && <div className="mt-0.5 text-sm text-slate-500">{propName}</div>}
                {description && <p className="mt-2 text-sm text-slate-600">{description}</p>}

                {(d.child_drafts?.length ?? 0) > 0 && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    {(d.child_drafts ?? []).map((c) => (
                      <span
                        key={c.id}
                        className="inline-flex items-center rounded-full border border-sky-200 bg-sky-50 px-2.5 py-0.5 text-xs font-medium text-sky-700"
                      >
                        {childLabel(c.entity_type)}
                      </span>
                    ))}
                  </div>
                )}

                {d.ai_reasoning && (
                  <details className="mt-3 rounded-md bg-slate-50 px-3 py-2">
                    <summary className="cursor-pointer text-xs font-medium text-slate-500 select-none">
                      AI reasoning
                    </summary>
                    <p className="mt-1.5 text-xs leading-relaxed text-slate-500">{d.ai_reasoning}</p>
                  </details>
                )}

                {tab === 'pending' && (
                  <>
                    {needsProperty[d.id] !== undefined && (
                      <div className="mt-3 rounded-md border border-amber-200 bg-amber-50 p-3">
                        <div className="text-xs font-medium text-amber-800">
                          A property is required to approve this draft. Pick one:
                        </div>
                        <select
                          value={propertyChoice[d.id] ?? ''}
                          onChange={(e) =>
                            setPropertyChoice((m) => ({ ...m, [d.id]: e.target.value }))
                          }
                          className={`${inputCls} mt-2 max-w-xs`}
                        >
                          <option value="">Select a property…</option>
                          {properties.map((p) => (
                            <option key={p.id} value={String(p.id)}>
                              {p.name}
                            </option>
                          ))}
                        </select>
                      </div>
                    )}
                    <div className="mt-4 flex gap-2">
                      <button
                        className={btnPrimary}
                        disabled={busyId === d.id || (needsProperty[d.id] !== undefined && !propertyChoice[d.id])}
                        onClick={() => void approve(d)}
                      >
                        {busyId === d.id ? 'Approving…' : needsProperty[d.id] !== undefined ? 'Approve with property' : 'Approve'}
                      </button>
                      <button className={btnSecondary} disabled={busyId === d.id} onClick={() => void reject(d)}>
                        Reject
                      </button>
                    </div>
                  </>
                )}
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
