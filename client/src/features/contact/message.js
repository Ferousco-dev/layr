// Builds the email a visitor sends. Swap sendMessage for an API call when a mail service exists.
export const MAX_MESSAGE = 4000

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/

export function validate({ name, email, message }) {
  const errors = {}
  if (!name.trim()) errors.name = 'Please tell us your name.'
  if (!EMAIL.test(email.trim())) errors.email = 'Enter a valid email address so we can reply.'
  if (message.trim().length < 10) errors.message = 'Please write a little more (at least 10 characters).'
  if (message.length > MAX_MESSAGE) errors.message = `Please keep your message under ${MAX_MESSAGE} characters.`
  return errors
}

// A name never carries line breaks, so it cannot add headers to the email.
const oneLine = (text) => text.replace(/[\u0000-\u001f\u007f]+/g, ' ').trim()

export function composeEmail({ name, email, message }) {
  const who = oneLine(name)
  const subject = `Layr contact from ${who}`
  const body = `${message.trim()}\n\n---\nName: ${who}\nReply to: ${oneLine(email)}\n`
  return { subject, body }
}

export function mailtoLink(to, fields) {
  const { subject, body } = composeEmail(fields)
  return `mailto:${to}?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`
}

// sendMessage opens the visitor's email app with the message ready to send.
export function sendMessage(to, fields) {
  window.location.href = mailtoLink(to, fields)
}
