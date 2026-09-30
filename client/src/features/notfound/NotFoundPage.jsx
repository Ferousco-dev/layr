import { Link } from '../../app/router.jsx'
import PageHeader from '../../components/PageHeader.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import './notfound.css'

export default function NotFoundPage() {
  return (
    <div className="page notfound-page">
      <PageHeader />
      <main className="notfound">
        <p className="code" aria-hidden="true">
          404
        </p>
        <h1>We can’t find that page</h1>
        <p className="lead">The link may be broken, or the page may have moved. Let’s get you back on track.</p>
        <div className="actions">
          <Link className="go" to="/">
            Go to Layr
          </Link>
          <Link className="alt" to="/contact">
            Contact us
          </Link>
        </div>
      </main>
      <SiteFooter />
    </div>
  )
}
