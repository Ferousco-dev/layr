import { Link } from '../app/router.jsx'
import Brand from './Brand.jsx'
import './header.css'

// PageHeader is the fixed top bar of public pages: the brand and a way back.
export default function PageHeader() {
  return (
    <header className="page-header">
      <Brand />
      <Link className="back" to="/">
        Back to Layr
      </Link>
    </header>
  )
}
