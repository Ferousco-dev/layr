import ProviderIcon from '../../../components/ProviderIcon.jsx'

export default function IntegrationsCard() {
  return (
    <section className="panel" aria-labelledby="integrations-title">
      <div className="section-heading">
        <h2 id="integrations-title">Integrations</h2>
        <p>Connect your external accounts.</p>
      </div>
      <div className="integration">
        <div className="provider-info">
          <span className="provider-icon">
            <ProviderIcon id="github" />
          </span>
          <div>
            <h3>GitHub</h3>
            <p>Connect your GitHub account to push generated code directly to your repositories.</p>
          </div>
        </div>
        <button className="secondary" type="button" disabled title="GitHub connection is not available yet">
          Coming soon
        </button>
      </div>
    </section>
  )
}
