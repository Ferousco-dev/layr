import { useEffect, useRef, useState } from 'react'
import { ApiError, DELETE_PHRASE, deleteAccount } from '../../../lib/api'
import Loader from '../../../components/Loader.jsx'

// DeleteAccountDialog asks for a typed phrase, then deletes the account and returns to the landing page.
export default function DeleteAccountDialog({ onClose }) {
  const dialog = useRef(null)
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    dialog.current.showModal()
  }, [])

  const confirm = async (event) => {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await deleteAccount()
      window.location.assign('/')
    } catch (err) {
      setBusy(false)
      setError(err instanceof ApiError ? err.message : 'Could not delete the account.')
    }
  }

  return (
    <dialog ref={dialog} className="confirm-dialog" onClose={onClose} aria-labelledby="delete-title">
      <form onSubmit={confirm}>
        <h2 id="delete-title">Delete account?</h2>
        <p>This permanently deletes your Layr account, your projects, imported designs and saved API keys. It cannot be undone.</p>
        <label htmlFor="delete-phrase">
          Type <strong>{DELETE_PHRASE}</strong> to confirm.
        </label>
        <input id="delete-phrase" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" spellCheck="false" disabled={busy} />
        {error && (
          <p className="form-status error" role="alert">
            {error}
          </p>
        )}
        <div className="dialog-actions">
          <button type="button" className="secondary neutral" onClick={() => dialog.current.close()} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="danger-button" disabled={busy || typed !== DELETE_PHRASE}>
            {busy ? <Loader size={20} /> : 'Delete my account'}
          </button>
        </div>
      </form>
    </dialog>
  )
}
