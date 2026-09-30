import { useEffect, useRef, useState } from 'react'
import { Link } from '../app/router.jsx'
import { initials } from '../lib/format.js'

export default function ProfileMenu({ user, onSignOut }) {
  const [open, setOpen] = useState(false)
  const box = useRef(null)

  useEffect(() => {
    if (!open) return
    const close = (e) => (e.type === 'keydown' ? e.key === 'Escape' && setOpen(false) : !box.current.contains(e.target) && setOpen(false))
    document.addEventListener('click', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('keydown', close)
    }
  }, [open])

  return (
    <div className="profile" ref={box}>
      <button className="profile-button" type="button" aria-expanded={open} aria-controls="profile-menu" onClick={() => setOpen(!open)}>
        {user.avatar_url ? <img src={user.avatar_url} alt="" referrerPolicy="no-referrer" /> : <span className="avatar-fallback big">{initials(user.name)}</span>}
        <span className="profile-name">{user.name}</span>
        <svg className="chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" aria-hidden="true">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>
      {open && (
        <div id="profile-menu" className="dropdown">
          {user.email && <p className="dropdown-email">{user.email}</p>}
          <Link to="/" onClick={() => setOpen(false)}>
            Projects
          </Link>
          <Link to="/profile" onClick={() => setOpen(false)}>
            Profile / Settings
          </Link>
          <button type="button" onClick={onSignOut}>
            Log out
          </button>
        </div>
      )}
    </div>
  )
}
