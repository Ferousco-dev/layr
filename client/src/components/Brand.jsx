import { Link } from '../app/router.jsx'
import Logo from './Logo.jsx'

export default function Brand({ small }) {
  return (
    <Link className={small ? 'brand small' : 'brand'} to="/" aria-label="Layr home">
      <Logo height={small ? 18 : 40} />
      <span>Layr</span>
    </Link>
  )
}
