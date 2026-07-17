import { useCallback, useEffect, useState } from 'react'
import { del, get, post, put, fmtDate } from '../lib/api'
import type { Property } from '../lib/types'
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

export default function Properties() {
  const [properties, setProperties] = useState<Property[] | null>(null)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<Property | 'new' | null>(null)
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [modalError, setModalError] = useState('')
  const [saveBusy, setSaveBusy] = useState(false)
  const [deleting, setDeleting] = useState<Property | null>(null)
  const [deleteBusy, setDeleteBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setProperties((await get<Property[]>('/api/properties')) ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load properties')
      setProperties([])
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const openNew = () => {
    setName('')
    setAddress('')
    setModalError('')
    setEditing('new')
  }

  const openEdit = (p: Property) => {
    setName(p.name)
    setAddress(p.address)
    setModalError('')
    setEditing(p)
  }

  const save = async () => {
    if (!name.trim()) {
      setModalError('Name is required')
      return
    }
    setSaveBusy(true)
    setModalError('')
    try {
      const body = { name: name.trim(), address: address.trim() }
      if (editing === 'new') await post('/api/properties', body)
      else if (editing) await put(`/api/properties/${editing.id}`, body)
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
      await del(`/api/properties/${deleting.id}`)
      setDeleting(null)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'delete failed')
      setDeleting(null)
    } finally {
      setDeleteBusy(false)
    }
  }

  return (
    <div className="p-8 bg-slate-50 min-h-screen">
      <PageHeader
        title="Properties"
        subtext="The buildings and homes in your portfolio."
        actions={
          <button className={btnPrimary} onClick={openNew}>
            + Add property
          </button>
        }
      />
      <ErrorBanner message={error} />

      {properties === null ? (
        <Spinner />
      ) : properties.length === 0 ? (
        <Card>
          <EmptyState
            title="No properties yet"
            hint="Add your first property to start tracking maintenance."
            action={
              <button className={btnPrimary} onClick={openNew}>
                + Add property
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
                <th className="px-5 py-3">Address</th>
                <th className="px-5 py-3">Added</th>
                <th className="px-5 py-3" />
              </tr>
            </thead>
            <tbody>
              {properties.map((p) => (
                <tr key={p.id} className="border-b border-slate-100 last:border-0 hover:bg-slate-50/60">
                  <td className="px-5 py-3 font-medium text-slate-900">{p.name}</td>
                  <td className="px-5 py-3 text-slate-600">{p.address || '—'}</td>
                  <td className="px-5 py-3 text-slate-500 whitespace-nowrap">{fmtDate(p.created_at)}</td>
                  <td className="px-5 py-3 text-right whitespace-nowrap">
                    <button className="text-sm text-sky-600 hover:underline" onClick={() => openEdit(p)}>
                      Edit
                    </button>
                    <button className="ml-3 text-sm text-red-500 hover:underline" onClick={() => setDeleting(p)}>
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
          title={editing === 'new' ? 'Add property' : `Edit ${editing.name}`}
          onClose={() => setEditing(null)}
          footer={
            <>
              <button className={btnSecondary} onClick={() => setEditing(null)}>
                Cancel
              </button>
              <button className={btnPrimary} disabled={saveBusy} onClick={() => void save()}>
                {saveBusy ? 'Saving…' : 'Save property'}
              </button>
            </>
          }
        >
          <ErrorBanner message={modalError} />
          <label className="block text-xs font-medium text-slate-600 mb-1">Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Maple Court Apartments"
            className={inputCls}
            autoFocus
          />
          <label className="mt-3 block text-xs font-medium text-slate-600 mb-1">Address</label>
          <input
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="412 Maple Ct, Springfield"
            className={inputCls}
          />
        </Modal>
      )}

      {deleting && (
        <Modal
          title="Delete property"
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
            Delete <span className="font-medium text-slate-900">{deleting.name}</span>? Tickets attached to it
            will lose their property link.
          </p>
        </Modal>
      )}
    </div>
  )
}
