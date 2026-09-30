import { useState } from 'react'
import PageHeader from '../../components/PageHeader.jsx'
import SiteFooter from '../../components/SiteFooter.jsx'
import { CONTACT_EMAIL } from '../../config/env.js'
import { composeEmail, MAX_MESSAGE, sendMessage, validate } from './message.js'
import './contact.css'

const EMPTY = { name: '', email: '', message: '' }

export default function ContactPage() {
  const [fields, setFields] = useState(EMPTY)
  const [errors, setErrors] = useState({})
  const [sent, setSent] = useState(false)
  const [copied, setCopied] = useState('')

  const change = (key) => (e) => {
    setFields({ ...fields, [key]: e.target.value })
    if (errors[key]) setErrors({ ...errors, [key]: undefined })
  }

  const submit = (event) => {
    event.preventDefault()
    const found = validate(fields)
    setErrors(found)
    if (Object.keys(found).length) {
      const first = ['name', 'email', 'message'].find((k) => found[k])
      document.getElementById(`contact-${first}`)?.focus()
      return
    }
    sendMessage(CONTACT_EMAIL, fields)
    setSent(true)
  }

  const copy = async (text, label) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(label)
      setTimeout(() => setCopied(''), 2500)
    } catch {
      setCopied('')
    }
  }

  const { subject, body } = composeEmail(fields)

  return (
    <div className="page contact-page">
      <PageHeader />
      <main className="contact">
        <div className="contact-intro">
          <h1>Contact us</h1>
          <p>Questions, feedback or a problem with an import? Send us a message and we will reply by email.</p>
          <p className="direct">
            Prefer email? Write to{' '}
            <a href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</a>{' '}
            <button type="button" className="copy" onClick={() => copy(CONTACT_EMAIL, 'address')}>
              {copied === 'address' ? 'Copied' : 'Copy'}
            </button>
          </p>
        </div>

        <form className="contact-form" onSubmit={submit} noValidate>
          <div className={errors.name ? 'field has-error' : 'field'}>
            <label htmlFor="contact-name">Your name</label>
            <input id="contact-name" value={fields.name} onChange={change('name')} autoComplete="name" aria-invalid={Boolean(errors.name)} aria-describedby={errors.name ? 'err-name' : undefined} />
            {errors.name && <p id="err-name" className="error">{errors.name}</p>}
          </div>

          <div className={errors.email ? 'field has-error' : 'field'}>
            <label htmlFor="contact-email">Your email</label>
            <input id="contact-email" type="email" value={fields.email} onChange={change('email')} autoComplete="email" aria-invalid={Boolean(errors.email)} aria-describedby={errors.email ? 'err-email' : undefined} />
            {errors.email && <p id="err-email" className="error">{errors.email}</p>}
          </div>

          <div className={errors.message ? 'field has-error' : 'field'}>
            <label htmlFor="contact-message">Message</label>
            <textarea id="contact-message" rows="7" value={fields.message} onChange={change('message')} aria-invalid={Boolean(errors.message)} aria-describedby={errors.message ? 'err-message' : undefined} />
            <p className="count" aria-hidden="true">
              {fields.message.length} / {MAX_MESSAGE}
            </p>
            {errors.message && <p id="err-message" className="error">{errors.message}</p>}
          </div>

          <button className="send" type="submit">
            Send message
          </button>

          {sent && (
            <div className="sent" role="status">
              <p>
                <strong>Your email app should be open with the message ready.</strong> Press Send there to finish. Nothing has been sent yet.
              </p>
              <p>
                Nothing opened? Copy the message and email it to <a href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</a>.
              </p>
              <div className="sent-actions">
                <button type="button" className="ghost" onClick={() => copy(`Subject: ${subject}\n\n${body}`, 'message')}>
                  {copied === 'message' ? 'Copied' : 'Copy message'}
                </button>
                <button type="button" className="ghost" onClick={() => sendMessage(CONTACT_EMAIL, fields)}>
                  Open email app again
                </button>
              </div>
            </div>
          )}
        </form>
      </main>
      <SiteFooter />
    </div>
  )
}
