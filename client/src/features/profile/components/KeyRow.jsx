import { useState } from 'react'
import { ApiError, deleteAiKey, saveAiKey } from '../../../lib/api'
import { timeAgo } from '../../../lib/format.js'
import Loader from '../../../components/Loader.jsx'
import { EyeIcon, EyeOffIcon } from '../../../components/icons.jsx'
import ProviderIcon from '../../../components/ProviderIcon.jsx'

// KeyRow is one provider: the saved key is never shown again, only its last four characters.
export default function KeyRow({ provider, status, onChange }) {
  const [value, setValue] = useState('')
  const [reveal, setReveal] = useState(false)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState({ kind: '', text: '' })

  const id = `${provider.id}-key`

  const save = async (event) => {
    event.preventDefault()
    if (!value.trim()) {
      setMessage({ kind: 'error', text: 'Enter a key first.' })
      return
    }
    setBusy('save')
    try {
      const next = await saveAiKey(provider.id, value.trim())
      onChange(next)
      setValue('')
      setReveal(false)
      setMessage({ kind: 'ok', text: `Saved. It ends in ${next.hint}.` })
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not save the key.' })
    } finally {
      setBusy('')
    }
  }

  const remove = async () => {
    setBusy('remove')
    try {
      await deleteAiKey(provider.id)
      onChange({ provider: provider.id, saved: false, hint: null, updated_at: null })
      setMessage({ kind: 'ok', text: 'Key removed.' })
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not remove the key.' })
    } finally {
      setBusy('')
    }
  }

  return (
    <form className="provider" onSubmit={save} noValidate>
      <div className="provider-info">
        <span className="provider-icon">
          <ProviderIcon id={provider.id} />
        </span>
        <div>
          <h3>{provider.name}</h3>
          <p>{provider.blurb}</p>
        </div>
      </div>

      <div className="key-field">
        <label className="sr-only" htmlFor={id}>
          {provider.name} API key
        </label>
        <input id={id} type={reveal ? 'text' : 'password'} value={value} onChange={(e) => setValue(e.target.value)} placeholder={status.saved ? `Saved, ends in ${status.hint}` : provider.placeholder} autoComplete="off" spellCheck="false" disabled={Boolean(busy)} />
        <button type="button" className="reveal" aria-label={`${reveal ? 'Hide' : 'Show'} ${provider.name} API key`} aria-pressed={reveal} onClick={() => setReveal(!reveal)}>
          {reveal ? <EyeOffIcon /> : <EyeIcon />}
        </button>
      </div>

      <div className="row-actions">
        <button className="primary" type="submit" disabled={Boolean(busy)}>
          {busy === 'save' ? <Loader size={20} /> : status.saved ? 'Replace key' : 'Save key'}
        </button>
        {status.saved && (
          <button className="ghost-danger" type="button" onClick={remove} disabled={Boolean(busy)}>
            {busy === 'remove' ? <Loader size={20} /> : 'Remove'}
          </button>
        )}
      </div>

      <p className={`form-status ${message.kind}`} role="status">
        {message.text || (status.saved ? `Key saved${status.updated_at ? `, updated ${timeAgo(status.updated_at)}` : ''}. It is stored encrypted and never shown again.` : '')}
      </p>
    </form>
  )
}
