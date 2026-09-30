import { useEffect, useRef, useState } from 'react'
import Loader from '../../../components/Loader.jsx'
import ProviderIcon from '../../../components/ProviderIcon.jsx'
import { EyeIcon, EyeOffIcon } from '../../../components/icons.jsx'
import { PROVIDERS } from '../../../lib/providers.js'

// AddKeyDialog appears when Generate is pressed and no AI key is saved yet.
export default function AddKeyDialog({ error, onSave, onCancel }) {
  const dialog = useRef(null)
  const [provider, setProvider] = useState(PROVIDERS[0].id)
  const [key, setKey] = useState('')
  const [reveal, setReveal] = useState(false)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState('')

  useEffect(() => {
    dialog.current.showModal()
  }, [])
  useEffect(() => setProblem(error), [error])

  const current = PROVIDERS.find((p) => p.id === provider)

  const submit = async (event) => {
    event.preventDefault()
    if (!key.trim()) return setProblem('Paste your API key first.')
    setBusy(true)
    setProblem('')
    await onSave(provider, key.trim())
    setBusy(false)
  }

  return (
    <dialog ref={dialog} className="flow-dialog" onClose={onCancel} aria-labelledby="add-key-title">
      <form onSubmit={submit} noValidate>
        <h2 id="add-key-title">Add an AI key to generate code</h2>
        <p className="lead">Layr writes code with your own AI key. It is stored encrypted and only its last four characters are ever shown again.</p>

        <fieldset className="choices">
          <legend className="sr-only">AI provider</legend>
          {PROVIDERS.map((p) => (
            <label key={p.id} className={provider === p.id ? 'choice on' : 'choice'}>
              <input type="radio" name="provider" value={p.id} checked={provider === p.id} onChange={() => setProvider(p.id)} disabled={busy} />
              <ProviderIcon id={p.id} size={34} />
              <span>
                <b>{p.name}</b>
                <small>{p.blurb}</small>
              </span>
            </label>
          ))}
        </fieldset>

        <label className="key-label" htmlFor="flow-key">
          {current.name} API key
        </label>
        <div className="key-input">
          <input id="flow-key" type={reveal ? 'text' : 'password'} value={key} onChange={(e) => setKey(e.target.value)} placeholder={current.placeholder} autoComplete="off" spellCheck="false" disabled={busy} />
          <button type="button" aria-label={reveal ? 'Hide key' : 'Show key'} aria-pressed={reveal} onClick={() => setReveal(!reveal)}>
            {reveal ? <EyeOffIcon /> : <EyeIcon />}
          </button>
        </div>
        {problem && (
          <p className="problem" role="alert">
            {problem}
          </p>
        )}

        <div className="dialog-actions">
          <button type="button" className="secondary" onClick={() => dialog.current.close()} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={busy}>
            {busy ? <Loader size={20} /> : 'Save and generate'}
          </button>
        </div>
      </form>
    </dialog>
  )
}
