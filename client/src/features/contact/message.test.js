import test from 'node:test'
import assert from 'node:assert/strict'
import { composeEmail, mailtoLink, validate } from './message.js'

const good = { name: 'Ada', email: 'ada@example.com', message: 'Hello, I have a question about imports.' }

test('a complete message passes', () => {
  assert.deepEqual(validate(good), {})
})

test('each field is checked', () => {
  const errors = validate({ name: ' ', email: 'nope', message: 'short' })
  assert.deepEqual(Object.keys(errors).sort(), ['email', 'message', 'name'])
  assert.ok(validate({ ...good, email: 'a@b' }).email)
  assert.ok(validate({ ...good, message: 'x'.repeat(4001) }).message)
})

test('the email carries the message and who sent it', () => {
  const { subject, body } = composeEmail(good)
  assert.equal(subject, 'Layr contact from Ada')
  assert.match(body, /question about imports/)
  assert.match(body, /Reply to: ada@example.com/)
})

test('the mailto link is encoded and cannot inject headers', () => {
  const link = mailtoLink('hello@layr.appmd.dev', { ...good, name: 'Ada\r\nBcc: evil@example.com', message: 'Line one\nLine two & more = ok' })
  assert.ok(link.startsWith('mailto:hello@layr.appmd.dev?subject='))
  assert.ok(!link.includes('\r') && !link.includes('\n'))
  assert.ok(!/[?&]bcc=/i.test(link.replace(/%0D%0A/gi, '')))
  assert.ok(link.includes('%26'))
  assert.ok(!link.includes('%0D%0ABcc') && link.includes('Ada%20Bcc'))
})
