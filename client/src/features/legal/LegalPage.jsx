import { Link } from '../../app/router.jsx'
import PageHeader from '../../components/PageHeader.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import { CONTACT_EMAIL, OPERATOR_NAME } from '../../config/env.js'
import { useState } from 'react'
import { useActiveSection, useWide } from './useActiveSection.js'
import './legal.css'

// Inline tokens let content link to the other page or the contact address without JSX in the data files.
function Inline({ text }) {
  return text.split(/(\{privacy\}|\{terms\}|\{email\}|\{operator\})/).map((part, i) => {
    if (part === '{privacy}') return <Link key={i} to="/privacy">Privacy Policy</Link>
    if (part === '{terms}') return <Link key={i} to="/terms">Terms and Conditions</Link>
    if (part === '{operator}') return OPERATOR_NAME
    if (part === '{email}') return CONTACT_EMAIL ? <a key={i} href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</a> : 'the contact details published on this website'
    return part
  })
}

function Block({ block }) {
  if (typeof block === 'string') return <p><Inline text={block} /></p>
  return (
    <ul>
      {block.list.map((item) => (
        <li key={item}>
          <Inline text={item} />
        </li>
      ))}
    </ul>
  )
}

export default function LegalPage({ doc }) {
  const ids = doc.sections.map((s) => s.id)
  const active = useActiveSection(ids)
  const wide = useWide()
  const [openSmall, setOpenSmall] = useState(false)

  return (
    <div className="page legal-page">
      <PageHeader />

      <div className="legal-layout">
        <nav className="toc" aria-label="Contents">
          <details open={wide || openSmall} onToggle={(e) => !wide && setOpenSmall(e.currentTarget.open)}>
            <summary onClick={(e) => wide && e.preventDefault()}>On this page</summary>
            <ol>
              {doc.sections.map((s) => (
                <li key={s.id}>
                  <a href={`#${s.id}`} className={s.id === active ? 'active' : undefined} aria-current={s.id === active ? 'true' : undefined}>
                    {s.title}
                  </a>
                </li>
              ))}
            </ol>
          </details>
        </nav>

        <article className="legal">
          <h1>{doc.title}</h1>
          <p className="meta">Effective {doc.effective}</p>

          <aside className="summary" aria-label="Summary">
            <h2>In short</h2>
            <ul>
              {doc.summary.map((line) => (
                <li key={line}>
                  <Inline text={line} />
                </li>
              ))}
            </ul>
          </aside>

          {doc.sections.map((s, i) => (
            <section key={s.id} id={s.id}>
              <h2>
                {i + 1}. {s.title}
              </h2>
              {s.body.map((block, j) => (
                <Block key={j} block={block} />
              ))}
            </section>
          ))}
        </article>
      </div>
      <SiteFooter />
    </div>
  )
}
