import test from 'node:test'
import assert from 'node:assert/strict'
import { authErrorMessage } from './messages.js'

test('known backend codes have their own text', () => {
  assert.match(authErrorMessage('OAUTH_STATE_INVALID'), /expired/)
  assert.match(authErrorMessage('DEPENDENCY_UNAVAILABLE'), /unavailable/)
})

test('unknown codes fall back to a generic message and never echo the code', () => {
  const text = authErrorMessage('<script>alert(1)</script>')
  assert.equal(text, authErrorMessage('OAUTH_CALLBACK_FAILED'))
  assert.ok(!text.includes('<'))
})

test('no code means no message', () => {
  assert.equal(authErrorMessage(''), '')
  assert.equal(authErrorMessage(null), '')
})
