import { loginUrl } from '../../../lib/api'
import { initials } from '../../../lib/format.js'
import { CheckIcon, FigmaMark, MailIcon } from '../../../components/icons.jsx'

const FIGMA = {
  active: { className: 'connected', text: 'Connected to Figma' },
  reconnect_required: { className: 'connected warn', text: 'Figma needs to be reconnected' },
  missing: { className: 'connected warn', text: 'Not connected to Figma' },
}

export default function IdentityCard({ user, figma }) {
  const badge = FIGMA[figma.status] || FIGMA.missing
  return (
    <section className="panel identity" aria-labelledby="profile-title">
      <div className="section-heading">
        <h1 id="profile-title">Profile</h1>
        <p>Your Figma account information.</p>
      </div>
      <div className="identity-details">
        {user.avatar_url ? <img className="portrait" src={user.avatar_url} alt={user.name} referrerPolicy="no-referrer" /> : <span className="portrait fallback">{initials(user.name)}</span>}
        <div>
          <h2>{user.name}</h2>
          {user.email && (
            <p className="identity-email">
              <MailIcon />
              {user.email}
            </p>
          )}
          <div className={badge.className}>
            <FigmaMark />
            {badge.text}
            {figma.status === 'active' ? (
              <span className="tick" aria-hidden="true">
                <CheckIcon />
              </span>
            ) : (
              <a href={loginUrl}>Reconnect</a>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
