import test from 'node:test'
import assert from 'node:assert/strict'
import { privacy } from './content/privacy.js'
import { terms } from './content/terms.js'

const docs = { terms, privacy }
const text = (doc) => JSON.stringify(doc)

for (const [name, doc] of Object.entries(docs)) {
  test(`${name}: sections have unique ids and titles`, () => {
    const ids = doc.sections.map((s) => s.id)
    assert.equal(new Set(ids).size, ids.length)
    for (const s of doc.sections) {
      assert.ok(s.title && s.body.length > 0, s.id)
    }
  })

  test(`${name}: only known inline tokens are used`, () => {
    const tokens = text(doc).match(/\{[a-z]+\}/g) || []
    for (const t of tokens) assert.ok(['{privacy}', '{terms}', '{email}', '{operator}'].includes(t), t)
  })

  test(`${name}: no em dashes`, () => {
    assert.ok(!text(doc).includes('—'))
  })
}

test('privacy states the facts the product really has', () => {
  const p = text(privacy)
  for (const fact of ['current_user:read', 'file_content:read', '30 days', '24 hours', '7 days', 'last four']) {
    assert.ok(p.includes(fact), fact)
  }
  assert.ok(p.includes('do not sell'))
})

test('terms cover keys, ownership and deletion', () => {
  const t = text(terms)
  for (const fact of ['AI provider keys', 'keep all rights', 'delete your account', 'as is']) {
    assert.ok(t.includes(fact), fact)
  }
})
