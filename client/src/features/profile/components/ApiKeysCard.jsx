import { PROVIDERS } from '../../../lib/providers.js'
import KeyRow from './KeyRow.jsx'

export default function ApiKeysCard({ keys, onChange }) {
  const byId = Object.fromEntries(keys.map((k) => [k.provider, k]))
  return (
    <section className="panel" aria-labelledby="keys-title">
      <div className="section-heading">
        <h2 id="keys-title">AI API Keys</h2>
        <p>Bring your own API keys(BYOK). Add the AI providers you want to use for code generation.</p>
      </div>
      <div className="providers">
        {PROVIDERS.map((p) => (
          <KeyRow key={p.id} provider={p} status={byId[p.id] || { provider: p.id, saved: false }} onChange={onChange} />
        ))}
      </div>
    </section>
  )
}
