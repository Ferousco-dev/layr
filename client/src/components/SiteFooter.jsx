import { Link } from '../app/router.jsx'
import Brand from './Brand.jsx'

// SiteFooter always sits at the bottom of the viewport, or after the content when the page is taller.
export default function SiteFooter({ withBrand = false }) {
  return (
    <footer className={withBrand ? 'site-footer with-brand' : 'site-footer'}>
      {withBrand && <Brand small />}
      <nav aria-label="Footer">
        <Link to="/docs">Docs</Link>
        <Link to="/privacy">Privacy</Link>
        <Link to="/terms">Terms &amp; Conditions</Link>
        <Link to="/contact">Contact</Link>
      </nav>
    </footer>
  )
}
