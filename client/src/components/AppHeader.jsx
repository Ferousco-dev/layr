import Brand from './Brand.jsx'
import ProfileMenu from './ProfileMenu.jsx'
import './header.css'

// AppHeader is the signed-in header: the brand on the left, the account menu on the right.
export default function AppHeader({ user, onSignOut }) {
  return (
    <header className="header">
      <Brand />
      <ProfileMenu user={user} onSignOut={onSignOut} />
    </header>
  )
}
