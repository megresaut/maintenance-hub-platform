import { useCallback, useEffect, useMemo, useState } from 'react'
import { del, get, post } from '../lib/api'
import { CATEGORIES, type PreferredVendor, type Property, type Vendor } from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
  PageHeader,
  Spinner,
  btnPrimary,
  inputCls,
} from '../components/ui'

export default function Preferred() {
  const [preferred, setPreferred] = useState<PreferredVendor[] | null>(null)
  const [properties, setProperties] = useState<Property[]>([])
  const [vendors, setVendors] = useState<Vendor[]>([])
  const [error, setError] = useState('')

  const [propertyId, setPropertyId] = useState('')
  const [category, setCategory] = useState('general')
  const [vendorId, setVendorId] = useState('')
  const [priority, setPriority] = useState('1')
  const [addBusy, setAddBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const [pref, props, vends] = await Promise.all([
        get<PreferredVendor[]>('/api/preferred-vendors'),
        get<Property[]>('/api/properties'),
        get<Vendor[]>('/api/vendors'),
      ])
      setPreferred(pref ?? [])
      setProperties(props ?? [])
      setVendors(vends ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load preferred vendors')
      setPreferred([])
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const categoryVendors = useMemo(
    () => vendors.filter((v) => v.category === category),
    [vendors, category],
  )

  // property_name -> category -> entries (priority-sorted)
  const grouped = useMemo(() => {
    const byProp = new Map<string, Map<string, PreferredVendor[]>>()
    for (const p of preferred ?? []) {
      const propKey = p.property_name || `Property #${p.property_id}`
      if (!byProp.has(propKey)) byProp.set(propKey, new Map())
      const byCat = byProp.get(propKey)!
      if (!byCat.has(p.category)) byCat.set(p.category, [])
      byCat.get(p.category)!.push(p)
    }
    for (const byCat of byProp.values()) {
      for (const list of byCat.values()) list.sort((a, b) => a.priority - b.priority)
    }
    return byProp
  }, [preferred])

  const add = async () => {
    if (!propertyId || !vendorId) {
      setError('Pick a property and a vendor')
      return
    }
    setAddBusy(true)
    setError('')
    try {
      await post('/api/preferred-vendors', {
        property_id: Number(propertyId),
        category,
        vendor_id: Number(vendorId),
        priority: Math.max(1, Number(priority) || 1),
      })
      setVendorId('')
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to add preferred vendor')
    } finally {
      setAddBusy(false)
    }
  }

  const remove = async (id: number) => {
    setError('')
    try {
      await del(`/api/preferred-vendors/${id}`)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to remove')
    }
  }

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Preferred Vendors"
        subtext="Per-property call lists — outreach tries these vendors first, in priority order."
      />
      <ErrorBanner message={error} />

      <Card className="p-5">
        <h2 className="text-sm font-semibold text-slate-900">Add to a preferred list</h2>
        <div className="mt-3 grid gap-3 sm:grid-cols-5">
          <div>
            <label className="block text-xs font-medium text-slate-600 mb-1">Property</label>
            <select value={propertyId} onChange={(e) => setPropertyId(e.target.value)} className={inputCls}>
              <option value="">Select…</option>
              {properties.map((p) => (
                <option key={p.id} value={String(p.id)}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 mb-1">Category</label>
            <select
              value={category}
              onChange={(e) => {
                setCategory(e.target.value)
                setVendorId('')
              }}
              className={inputCls}
            >
              {CATEGORIES.map((c) => (
                <option key={c} value={c}>
                  {c.replace(/_/g, ' ')}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 mb-1">Vendor</label>
            <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={inputCls}>
              <option value="">
                {categoryVendors.length ? 'Select…' : 'No vendors in category'}
              </option>
              {categoryVendors.map((v) => (
                <option key={v.id} value={String(v.id)}>
                  {v.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium text-slate-600 mb-1">Priority</label>
            <input
              type="number"
              min={1}
              value={priority}
              onChange={(e) => setPriority(e.target.value)}
              className={inputCls}
            />
          </div>
          <div className="flex items-end">
            <button className={`${btnPrimary} w-full`} disabled={addBusy} onClick={() => void add()}>
              {addBusy ? 'Adding…' : 'Add'}
            </button>
          </div>
        </div>
      </Card>

      {preferred === null ? (
        <Spinner />
      ) : grouped.size === 0 ? (
        <Card className="mt-6">
          <EmptyState
            title="No preferred vendors yet"
            hint="Add vendors above so outreach knows who to call first at each property."
          />
        </Card>
      ) : (
        <div className="mt-6 space-y-4">
          {[...grouped.entries()].map(([propName, byCat]) => (
            <Card key={propName} className="p-5">
              <h3 className="text-base font-semibold text-slate-900">{propName}</h3>
              <div className="mt-3 space-y-3">
                {[...byCat.entries()].map(([cat, entries]) => (
                  <div key={cat} className="flex flex-wrap items-center gap-2">
                    <span className="w-28 shrink-0 text-xs font-medium uppercase tracking-wide text-slate-500 capitalize">
                      {cat.replace(/_/g, ' ')}
                    </span>
                    {entries.map((e) => (
                      <span
                        key={e.id}
                        className="inline-flex items-center gap-1.5 rounded-full border border-sky-200 bg-sky-50 py-1 pl-1.5 pr-2 text-sm text-sky-800"
                      >
                        <span className="flex h-5 w-5 items-center justify-center rounded-full bg-sky-600 text-[10px] font-semibold text-white">
                          {e.priority}
                        </span>
                        {e.vendor_name}
                        <button
                          className="ml-0.5 text-sky-400 hover:text-sky-700"
                          title="Remove from list"
                          onClick={() => void remove(e.id)}
                        >
                          ✕
                        </button>
                      </span>
                    ))}
                  </div>
                ))}
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
