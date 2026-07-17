import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { get } from '../lib/api'
import type { Property, ScheduleItem } from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
  PageHeader,
  PriorityBadge,
  Spinner,
  StatusBadge,
  btnSecondary,
  inputCls,
} from '../components/ui'

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

const KIND_PILL: Record<ScheduleItem['kind'], string> = {
  work_order: 'bg-sky-100 text-sky-800',
  fto: 'bg-violet-100 text-violet-800',
  recurring: 'bg-emerald-100 text-emerald-800',
  calendar_event: 'bg-slate-200 text-slate-700',
}

const KIND_DOT: Record<ScheduleItem['kind'], string> = {
  work_order: 'bg-sky-500',
  fto: 'bg-violet-500',
  recurring: 'bg-emerald-500',
  calendar_event: 'bg-slate-400',
}

const KIND_LABEL: Record<ScheduleItem['kind'], string> = {
  work_order: 'Work order',
  fto: 'Internal (FTO)',
  recurring: 'Recurring',
  calendar_event: 'Calendar event',
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function ymd(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** Date-only strings are treated as local dates so items land on the right day. */
function parseStart(s: string): Date {
  return /^\d{4}-\d{2}-\d{2}$/.test(s) ? new Date(`${s}T00:00:00`) : new Date(s)
}

function fmtTime(d: Date): string {
  return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

function itemHref(it: ScheduleItem): string | null {
  if (it.kind === 'work_order') return `/work-orders/${it.id}`
  if (it.kind === 'fto') return `/tickets/fto/${it.id}`
  return null
}

function ItemPill({ item }: { item: ScheduleItem }) {
  const d = parseStart(item.start_at)
  const prefix =
    item.date_source === 'event' ? fmtTime(d) : item.date_source === 'due' ? 'due' : ''
  return (
    <div
      className={`truncate rounded px-1.5 py-0.5 text-[11px] font-medium leading-4 ${KIND_PILL[item.kind]}`}
      title={item.name}
    >
      {prefix && <span className="opacity-70">{prefix} </span>}
      {item.name}
    </div>
  )
}

function DayDetail({ dayKey, items, onClose }: { dayKey: string; items: ScheduleItem[]; onClose: () => void }) {
  const label = new Date(`${dayKey}T00:00:00`).toLocaleDateString(undefined, {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  })
  return (
    <Card className="w-full lg:w-80 shrink-0 self-start">
      <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3">
        <div className="text-sm font-semibold text-slate-900">{label}</div>
        <button onClick={onClose} className="text-slate-400 hover:text-slate-600" aria-label="Close">
          ✕
        </button>
      </div>
      <div className="max-h-[32rem] divide-y divide-slate-100 overflow-y-auto">
        {items.length === 0 ? (
          <EmptyState title="Nothing scheduled" hint="No items fall on this day." />
        ) : (
          items.map((it) => {
            const start = parseStart(it.start_at)
            const end = it.end_at ? parseStart(it.end_at) : null
            const href = itemHref(it)
            const body = (
              <>
                <div className="flex items-center gap-2">
                  <span className={`h-2 w-2 shrink-0 rounded-full ${KIND_DOT[it.kind]}`} />
                  <span className="truncate text-sm font-medium text-slate-900">{it.name}</span>
                </div>
                <div className="mt-1 text-xs text-slate-500">
                  {KIND_LABEL[it.kind]}
                  {it.property_name ? ` · ${it.property_name}` : ''}
                  {it.vendor_name ? ` · ${it.vendor_name}` : ''}
                </div>
                {it.date_source === 'event' && (
                  <div className="mt-1 text-xs text-slate-500">
                    {fmtTime(start)}
                    {end ? ` – ${fmtTime(end)}` : ''}
                  </div>
                )}
                {it.date_source === 'due' && <div className="mt-1 text-xs text-amber-600">Due date</div>}
                {(it.status || it.priority) && (
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {it.status && <StatusBadge status={it.status} />}
                    {it.priority && <PriorityBadge priority={it.priority} />}
                  </div>
                )}
              </>
            )
            return href ? (
              <Link key={`${it.kind}-${it.id}`} to={href} className="block px-4 py-3 hover:bg-sky-50/40 transition-colors">
                {body}
              </Link>
            ) : (
              <div key={`${it.kind}-${it.id}`} className="px-4 py-3">
                {body}
              </div>
            )
          })
        )}
      </div>
    </Card>
  )
}

export default function CalendarPage() {
  const today = new Date()
  const [month, setMonth] = useState(() => new Date(today.getFullYear(), today.getMonth(), 1))
  const [items, setItems] = useState<ScheduleItem[] | null>(null)
  const [properties, setProperties] = useState<Property[]>([])
  const [propertyFilter, setPropertyFilter] = useState('')
  const [selectedDay, setSelectedDay] = useState<string | null>(null)
  const [error, setError] = useState('')

  // Grid range: full weeks (Sun–Sat) covering the visible month.
  const { gridStart, cellCount } = useMemo(() => {
    const first = new Date(month.getFullYear(), month.getMonth(), 1)
    const start = new Date(first)
    start.setDate(first.getDate() - first.getDay())
    const daysInMonth = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate()
    const count = Math.ceil((first.getDay() + daysInMonth) / 7) * 7
    return { gridStart: start, cellCount: count }
  }, [month])

  useEffect(() => {
    get<Property[]>('/api/properties')
      .then((p) => setProperties(p ?? []))
      .catch(() => {
        /* filter dropdown stays empty */
      })
  }, [])

  useEffect(() => {
    let cancelled = false
    setItems(null)
    const from = ymd(gridStart)
    const last = new Date(gridStart)
    last.setDate(gridStart.getDate() + cellCount - 1)
    const to = ymd(last)
    get<ScheduleItem[]>(`/api/schedule?from=${from}&to=${to}`)
      .then((data) => {
        if (cancelled) return
        setItems(data ?? [])
        setError('')
      })
      .catch((e: unknown) => {
        if (cancelled) return
        setItems([])
        setError(e instanceof Error ? e.message : 'failed to load schedule')
      })
    return () => {
      cancelled = true
    }
  }, [gridStart, cellCount])

  const filtered = useMemo(() => {
    const all = items ?? []
    if (!propertyFilter) return all
    return all.filter((it) => String(it.property_id ?? '') === propertyFilter)
  }, [items, propertyFilter])

  const byDay = useMemo(() => {
    const map = new Map<string, ScheduleItem[]>()
    for (const it of filtered) {
      const key = ymd(parseStart(it.start_at))
      const arr = map.get(key)
      if (arr) arr.push(it)
      else map.set(key, [it])
    }
    for (const arr of map.values()) {
      arr.sort((a, b) => parseStart(a.start_at).getTime() - parseStart(b.start_at).getTime())
    }
    return map
  }, [filtered])

  const upcoming = useMemo(() => {
    const startOfToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()
    return [...filtered]
      .filter((it) => parseStart(it.start_at).getTime() >= startOfToday)
      .sort((a, b) => parseStart(a.start_at).getTime() - parseStart(b.start_at).getTime())
      .slice(0, 8)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtered])

  const todayKey = ymd(today)
  const monthLabel = month.toLocaleDateString(undefined, { month: 'long', year: 'numeric' })

  const cells: Date[] = []
  for (let i = 0; i < cellCount; i++) {
    const d = new Date(gridStart)
    d.setDate(gridStart.getDate() + i)
    cells.push(d)
  }

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Calendar"
        subtext="Everything with a date — work orders, internal jobs, recurring maintenance, and calendar events."
      />
      <ErrorBanner message={error} />

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <button
            className={btnSecondary}
            aria-label="Previous month"
            onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}
          >
            ‹
          </button>
          <button
            className={btnSecondary}
            aria-label="Next month"
            onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))}
          >
            ›
          </button>
          <button
            className={btnSecondary}
            onClick={() => setMonth(new Date(today.getFullYear(), today.getMonth(), 1))}
          >
            Today
          </button>
          <h2 className="ml-2 text-lg font-semibold text-slate-900">{monthLabel}</h2>
        </div>
        <select
          value={propertyFilter}
          onChange={(e) => setPropertyFilter(e.target.value)}
          className={`${inputCls} w-52`}
        >
          <option value="">All properties</option>
          {properties.map((p) => (
            <option key={p.id} value={String(p.id)}>
              {p.name}
            </option>
          ))}
        </select>
      </div>

      <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
        <Card className="min-w-0 flex-1 overflow-hidden">
          {items === null ? (
            <Spinner label="Loading schedule…" />
          ) : (
            <>
              <div className="grid grid-cols-7 border-b border-slate-200 bg-slate-50/60">
                {WEEKDAYS.map((d) => (
                  <div
                    key={d}
                    className="px-2 py-2 text-center text-xs font-medium uppercase tracking-wide text-slate-500"
                  >
                    {d}
                  </div>
                ))}
              </div>
              <div className="grid grid-cols-7">
                {cells.map((d) => {
                  const key = ymd(d)
                  const inMonth = d.getMonth() === month.getMonth()
                  const isToday = key === todayKey
                  const dayItems = byDay.get(key) ?? []
                  const extra = dayItems.length - 3
                  return (
                    <button
                      key={key}
                      onClick={() => setSelectedDay(key)}
                      className={`min-h-24 border-b border-r border-slate-100 p-1.5 text-left align-top transition-colors last:border-r-0 hover:bg-sky-50/40 ${
                        inMonth ? 'bg-white' : 'bg-slate-50/70'
                      } ${isToday ? 'ring-2 ring-inset ring-sky-400' : ''} ${
                        selectedDay === key ? 'bg-sky-50/60' : ''
                      }`}
                    >
                      <div
                        className={`mb-1 text-xs font-medium ${
                          isToday
                            ? 'inline-flex h-5 w-5 items-center justify-center rounded-full bg-sky-600 text-white'
                            : inMonth
                              ? 'text-slate-700'
                              : 'text-slate-300'
                        }`}
                      >
                        {d.getDate()}
                      </div>
                      <div className="space-y-0.5">
                        {dayItems.slice(0, 3).map((it) => (
                          <ItemPill key={`${it.kind}-${it.id}-${it.start_at}`} item={it} />
                        ))}
                        {extra > 0 && (
                          <div className="px-1.5 text-[11px] font-medium text-slate-400">+{extra} more</div>
                        )}
                      </div>
                    </button>
                  )
                })}
              </div>
              <div className="flex flex-wrap items-center gap-4 border-t border-slate-100 px-4 py-2.5">
                {(Object.keys(KIND_LABEL) as ScheduleItem['kind'][]).map((k) => (
                  <span key={k} className="inline-flex items-center gap-1.5 text-xs text-slate-500">
                    <span className={`h-2.5 w-2.5 rounded-sm ${KIND_DOT[k]}`} />
                    {KIND_LABEL[k]}
                  </span>
                ))}
              </div>
            </>
          )}
        </Card>

        {selectedDay && (
          <DayDetail
            dayKey={selectedDay}
            items={byDay.get(selectedDay) ?? []}
            onClose={() => setSelectedDay(null)}
          />
        )}
      </div>

      <Card className="mt-6 overflow-hidden">
        <div className="border-b border-slate-100 px-5 py-3 text-sm font-semibold text-slate-900">Upcoming</div>
        {items === null ? (
          <Spinner />
        ) : upcoming.length === 0 ? (
          <EmptyState title="Nothing upcoming" hint="No scheduled items from today onward in this month." />
        ) : (
          <div className="divide-y divide-slate-100">
            {upcoming.map((it) => {
              const start = parseStart(it.start_at)
              const href = itemHref(it)
              const row = (
                <div className="flex items-center gap-3 px-5 py-3">
                  <span className={`h-2 w-2 shrink-0 rounded-full ${KIND_DOT[it.kind]}`} />
                  <div className="w-32 shrink-0 text-sm text-slate-500">
                    {start.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })}
                    {it.date_source === 'event' && (
                      <span className="block text-xs text-slate-400">{fmtTime(start)}</span>
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-slate-900">
                      {it.date_source === 'due' && <span className="mr-1 text-xs font-normal text-amber-600">due</span>}
                      {it.name}
                    </div>
                    <div className="truncate text-xs text-slate-500">
                      {it.property_name ?? '—'}
                      {it.vendor_name ? ` · ${it.vendor_name}` : ''}
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    {it.status && <StatusBadge status={it.status} />}
                    {it.priority && <PriorityBadge priority={it.priority} />}
                  </div>
                </div>
              )
              return href ? (
                <Link
                  key={`${it.kind}-${it.id}-${it.start_at}`}
                  to={href}
                  className="block hover:bg-sky-50/40 transition-colors"
                >
                  {row}
                </Link>
              ) : (
                <div key={`${it.kind}-${it.id}-${it.start_at}`}>{row}</div>
              )
            })}
          </div>
        )}
      </Card>
    </div>
  )
}
