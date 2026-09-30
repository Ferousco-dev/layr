import { useEffect, useState } from 'react'
import { loginUrl } from '../../lib/api'
import Loader from '../../components/Loader.jsx'
import Logo from '../../components/Logo.jsx'
import { Link } from '../../app/router.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import { ArrowRight, FigmaMark } from '../../components/icons.jsx'
import { authErrorMessage, UNREACHABLE } from './messages.js'
import './landing.css'

export default function Landing({ checking, unreachable }) {
  const [notice] = useState(() => authErrorMessage(new URLSearchParams(window.location.search).get('auth_error')))

  useEffect(() => {
    if (window.location.search) window.history.replaceState(null, '', window.location.pathname)
  }, [])

  const message = notice || (unreachable ? UNREACHABLE : '')

  return (
    <div className="page landing">
      <div className="artwork" aria-hidden="true" />
      <main className="hero">
        <div className="logo">
          <Logo height={84} />
        </div>
        <h1>
          Turn your designs
          <br />
          <span>into real code</span>
        </h1>
        <p>
          Connect your Figma account to get started
          <br className="desktop-break" /> and turn your designs into production-ready code.
        </p>

        <div className="action">
          {checking ? (
            <Loader size={48} label="Checking your session" />
          ) : (
            <a className="login" href={loginUrl}>
              <FigmaMark />
              <span>Login with Figma</span>
              <ArrowRight />
            </a>
          )}
        </div>

        {!checking && (
          <p className="consent">
            By continuing, you agree to our <Link to="/terms">Terms</Link> and <Link to="/privacy">Privacy Policy</Link>.
          </p>
        )}

        {message && (
          <p className="status" role="status">
            {message}
          </p>
        )}
      </main>
      <SiteFooter />
    </div>
  )
}
