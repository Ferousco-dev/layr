import { Link } from '../../app/router.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'

export default function InfoPage({ title }) {
  return (
    <>
      <article className="doc">
        <Link to="/">← Back to Layr</Link>
        <h1>{title}</h1>
        <p>This page is a placeholder. Add your project’s {title.toLowerCase()} content before publishing.</p>
      </article>
      <SiteFooter />
    </>
  )
}
