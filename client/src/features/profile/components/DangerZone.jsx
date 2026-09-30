import { useState } from 'react'
import { LogoutIcon, TrashIcon } from '../../../components/icons.jsx'
import DeleteAccountDialog from './DeleteAccountDialog.jsx'

export default function DangerZone({ onSignOut }) {
  const [deleting, setDeleting] = useState(false)
  return (
    <section className="panel" aria-labelledby="danger-title">
      <div className="section-heading">
        <h2 id="danger-title">Danger Zone</h2>
        <p>Permanent and irreversible actions.</p>
      </div>
      <div className="account-actions">
        <div className="danger">
          <span className="action-icon">
            <TrashIcon />
          </span>
          <div>
            <h3>Delete account</h3>
            <p>This will permanently delete your Layr account and all your data. This action cannot be undone.</p>
          </div>
          <button type="button" className="danger-button" onClick={() => setDeleting(true)}>
            Delete account
          </button>
        </div>
        <div className="logout">
          <span className="action-icon">
            <LogoutIcon />
          </span>
          <div>
            <h3>Logout</h3>
            <p>Sign out from your Layr account.</p>
          </div>
          <button type="button" className="secondary neutral" onClick={onSignOut}>
            Logout
          </button>
        </div>
      </div>
      {deleting && <DeleteAccountDialog onClose={() => setDeleting(false)} />}
    </section>
  )
}
