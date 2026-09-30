import AppHeader from '../../components/AppHeader.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import ApiKeysCard from './components/ApiKeysCard.jsx'
import DangerZone from './components/DangerZone.jsx'
import IdentityCard from './components/IdentityCard.jsx'
import IntegrationsCard from './components/IntegrationsCard.jsx'
import ProfileSkeleton from './components/ProfileSkeleton.jsx'
import { useProfile } from './hooks/useProfile.js'
import './profile.css'

export default function Profile({ user, signOut }) {
  const { status, profile, error, reload, setKey } = useProfile()

  return (
    <div className="page profile-page">
      <AppHeader user={user} onSignOut={signOut} />
      <main className="profile-main">
        {status === 'loading' && <ProfileSkeleton />}
        {status === 'error' && (
          <p className="profile-error" role="alert">
            {error}{' '}
            <button type="button" className="link" onClick={reload}>
              Try again
            </button>
          </p>
        )}
        {status === 'ready' && (
          <>
            <IdentityCard user={profile.user} figma={profile.figma} />
            <ApiKeysCard keys={profile.ai_keys} onChange={setKey} />
            <IntegrationsCard />
            <DangerZone onSignOut={signOut} />
          </>
        )}
      </main>
      <SiteFooter withBrand />
    </div>
  )
}
