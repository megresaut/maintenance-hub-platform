import { useCallback, useEffect, useState } from 'react'
import { del, get, post, put } from '../lib/api'
import { CATEGORIES, type Vendor } from '../lib/types'
import {
  Card,
  EmptyState,
  ErrorBanner,
  Modal,
  PageHeader,
  Spinner,
  btnDanger,
  btnPrimary,
  btnSecondary,
  inputCls,
} from '../components/ui'

type VendorForm = {
  name: string
  category: string
  service_area_tags: string
  primary_email: string
  phone: string
  alt_email: string
  alt_phone: string
  website: string
  address: string
  notes: string
}

const emptyForm: VendorForm = {
  name: '',
  category: 'general',
  service_area_tags: '',
  primary_email: '',
  phone: '',
  alt_email: '',
  alt_phone: '',
  website: '',
  address: '',
  notes: '',
}

function toForm(v: Vendor): VendorForm {
  return {
    name: v.name,
    category: v.category,
    service_area_tags: (v.service_area_tags ?? []).join(', '),
    primary_email: v.primary_email ?? '',
    phone: v.phone ?? '',
    alt_email: v.alt_email ?? '',
    alt_phone: v.alt_phone ?? '',
    website: v.website ?? '',
    address: v.address ?? '',
    notes: v.notes ?? '',
  }
}

function toPayload(f: VendorForm) {
  return {
    name: f.name.trim(),
    category: f.category,
    service_area_tags: f.service_area_tags
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
    primary_email: f.primary_email.trim() || undefined,
    phone: f.phone.trim() || undefined,
    alt_email: f.alt_email.trim() || undefined,
    alt_phone: f.alt_phone.trim() || undefined,
    website: f.website.trim() || undefined,
    address: f.address.trim() || undefined,
    notes: f.notes.trim() || undefined,
  }
}

export default function Vendors() {
  const [vendors, setVendors] = useState<Vendor[] | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Vendor | 'new' | null>(null)
  const [form, setForm] = useState<VendorForm>(emptyForm)
  const [saveBusy, setSaveBusy] = useState(false)
  const [modalError, setModalError] = useState('')
  const [deleting, setDeleting] = useState<Vendor | null>(null)
  const [deleteBusy, setDeleteBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setVendors((await get<Vendor[]>('/api/vendors')) ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load vendors')
      setVendors([])
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const openNew = () => {
    setForm(emptyForm)
    setModalError('')
    setEditing('new')
  }

  const openEdit = (v: Vendor) => {
    setForm(toForm(v))
    setModalError('')
    setEditing(v)
  }

  const save = async () => {
    if (!form.name.trim()) {
      setModalError('Name is required')
      return
    }
    setSaveBusy(true)
    setModalError('')
    try {
      if (editing === 'new') await post('/api/vendors', toPayload(form))
      else if (editing) await put(`/api/vendors/${editing.id}`, toPayload(form))
      setEditing(null)
      await load()
    } catch (e) {
      setModalError(e instanceof Error ? e.message : 'save failed')
    } finally {
      setSaveBusy(false)
    }
  }

  const confirmDelete = async () => {
    if (!deleting) return
    setDeleteBusy(true)
    try {
      await del(`/api/vendors/${deleting.id}`)
      setDeleting(null)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'delete failed')
      setDeleting(null)
    } finally {
      setDeleteBusy(false)
    }
  }

  const field = (label: string, key: keyof VendorForm, placeholder = '') => (
    <div>
      <label className="block text-xs font-medium text-slate-600 mb-1">{label}</label>
      <input
        value={form[key]}
        onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))}
        placeholder={placeholder}
        className={inputCls}
      />
    </div>
  )

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Vendors"
        subtext="Your vendor directory — the pool used for outreach and preferred lists."
        actions={
          <button className={btnPrimary} onClick={openNew}>
            + Add vendor
          </button>
        }
      />
      <ErrorBanner message={error} />

      {vendors === null ? (
        <Spinner />
      ) : vendors.length === 0 ? (
        <Card>
          <EmptyState
            title="No vendors yet"
            hint="Add your first vendor to start sourcing quotes."
            action={
              <button className={btnPrimary} onClick={openNew}>
                + Add vendor
              </button>
            }
          />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-left text-xs font-medium uppercase tracking-wide text-slate-500">
                <th className="px-5 py-3">Name</th>
                <th className="px-5 py-3">Category</th>
                <th className="px-5 py-3">Service areas</th>
                <th className="px-5 py-3">Phone</th>
                <th className="px-5 py-3">Email</th>
                <th className="px-5 py-3" />
              </tr>
            </thead>
            <tbody>
              {vendors.map((v) => (
                <tr key={v.id} className="border-b border-slate-100 last:border-0 hover:bg-slate-50/60">
                  <td className="px-5 py-3 font-medium text-slate-900">{v.name}</td>
                  <td className="px-5 py-3">
                    <span className="inline-flex items-center rounded-full bg-sky-50 px-2.5 py-0.5 text-xs font-medium text-sky-700 capitalize">
                      {v.category.replace(/_/g, ' ')}
                    </span>
                  </td>
                  <td className="px-5 py-3">
                    <div className="flex flex-wrap gap-1">
                      {(v.service_area_tags ?? []).length === 0 ? (
                        <span className="text-slate-300">—</span>
                      ) : (
                        (v.service_area_tags ?? []).map((t) => (
                          <span
                            key={t}
                            className="inline-flex items-center rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600"
                          >
                            {t}
                          </span>
                        ))
                      )}
                    </div>
                  </td>
                  <td className="px-5 py-3 text-slate-600 whitespace-nowrap">{v.phone || '—'}</td>
                  <td className="px-5 py-3 text-slate-600">{v.primary_email || '—'}</td>
                  <td className="px-5 py-3 text-right whitespace-nowrap">
                    <button className="text-sm text-sky-600 hover:underline" onClick={() => openEdit(v)}>
                      Edit
                    </button>
                    <button className="ml-3 text-sm text-red-500 hover:underline" onClick={() => setDeleting(v)}>
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}

      {editing !== null && (
        <Modal
          title={editing === 'new' ? 'Add vendor' : `Edit ${editing.name}`}
          onClose={() => setEditing(null)}
          wide
          footer={
            <>
              <button className={btnSecondary} onClick={() => setEditing(null)}>
                Cancel
              </button>
              <button className={btnPrimary} disabled={saveBusy} onClick={() => void save()}>
                {saveBusy ? 'Saving…' : 'Save vendor'}
              </button>
            </>
          }
        >
          <ErrorBanner message={modalError} />
          <div className="grid gap-3 sm:grid-cols-2">
            {field('Name', 'name', 'Ace Plumbing Co.')}
            <div>
              <label className="block text-xs font-medium text-slate-600 mb-1">Category</label>
              <select
                value={form.category}
                onChange={(e) => setForm((f) => ({ ...f, category: e.target.value }))}
                className={inputCls}
              >
                {CATEGORIES.map((c) => (
                  <option key={c} value={c}>
                    {c.replace(/_/g, ' ')}
                  </option>
                ))}
              </select>
            </div>
            {field('Phone', 'phone', '+1 555 0100')}
            {field('Primary email', 'primary_email', 'dispatch@vendor.com')}
            {field('Alt phone', 'alt_phone')}
            {field('Alt email', 'alt_email')}
            {field('Website', 'website', 'https://…')}
            {field('Address', 'address')}
          </div>
          <div className="mt-3">
            {field('Service area tags (comma-separated)', 'service_area_tags', 'downtown, north side, 94110')}
          </div>
          <div className="mt-3">
            <label className="block text-xs font-medium text-slate-600 mb-1">Notes</label>
            <textarea
              value={form.notes}
              onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))}
              rows={2}
              className={inputCls}
            />
          </div>
        </Modal>
      )}

      {deleting && (
        <Modal
          title="Delete vendor"
          onClose={() => setDeleting(null)}
          footer={
            <>
              <button className={btnSecondary} onClick={() => setDeleting(null)}>
                Cancel
              </button>
              <button className={btnDanger} disabled={deleteBusy} onClick={() => void confirmDelete()}>
                {deleteBusy ? 'Deleting…' : 'Delete'}
              </button>
            </>
          }
        >
          <p className="text-sm text-slate-600">
            Delete <span className="font-medium text-slate-900">{deleting.name}</span>? This removes them from
            preferred lists and future outreach.
          </p>
        </Modal>
      )}
    </div>
  )
}
