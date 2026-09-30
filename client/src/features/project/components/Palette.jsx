import { useState } from 'react'

const WEIGHTS = { 100: 'Thin', 200: 'ExtraLight', 300: 'Light', 400: 'Regular', 500: 'Medium', 600: 'SemiBold', 700: 'Bold', 800: 'ExtraBold', 900: 'Black' }
const weightName = (w) => WEIGHTS[w] || String(w)

export function Colors({ colors, onNotice }) {
  const [copied, setCopied] = useState('')
  if (!colors.length) return null
  const copy = async (hex) => {
    try {
      await navigator.clipboard.writeText(hex)
      setCopied(hex)
      onNotice(`Copied ${hex}`)
      setTimeout(() => setCopied(''), 1800)
    } catch {
      onNotice(`Color ${hex}. Copy it manually.`)
    }
  }
  return (
    <section className="palette">
      <h3>Colors ({colors.length})</h3>
      <div className="swatches">
        {colors.slice(0, 16).map((c) => (
          <button key={`${c.hex}-${c.alpha}`} type="button" className="swatch" aria-label={`Copy color ${c.hex}`} onClick={() => copy(c.hex)}>
            <span style={{ background: c.hex, opacity: c.alpha < 1 ? Math.max(c.alpha, 0.15) : 1 }} />
            <small>{copied === c.hex ? 'Copied' : c.hex}</small>
          </button>
        ))}
      </div>
    </section>
  )
}

export function Fonts({ fonts }) {
  if (!fonts.length) return null
  return (
    <section className="fonts">
      <h3>Fonts ({fonts.length})</h3>
      <div className="font-grid">
        {fonts.slice(0, 6).map((f) => (
          <div key={f.family}>
            <b style={{ fontFamily: `"${f.family}", system-ui, sans-serif` }}>Aa</b>
            <span>
              {f.family}
              {f.weights.length > 0 && <small>{f.weights.map(weightName).join(' · ')}</small>}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}
