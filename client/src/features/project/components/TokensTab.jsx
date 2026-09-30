import { useState } from 'react'

const WEIGHTS = { 100: 'Thin', 200: 'ExtraLight', 300: 'Light', 400: 'Regular', 500: 'Medium', 600: 'SemiBold', 700: 'Bold', 800: 'ExtraBold', 900: 'Black' }
const weightName = (w) => (w == null ? 'Regular' : WEIGHTS[w] || String(w))

const shadowCss = (s) => {
  const alpha = s.alpha == null ? 1 : s.alpha
  const hex = s.hex || '#000000'
  const rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16)).join(',')
  return `${s.type === 'inner' ? 'inset ' : ''}${s.x}px ${s.y}px ${s.blur}px ${s.spread}px rgba(${rgb},${alpha})`
}

// TokensTab lists every value the design uses: colors, text styles, spacing, corner radii and shadows.
export default function TokensTab({ tokens }) {
  const [copied, setCopied] = useState('')
  if (!tokens) return <p className="tab-note">No design tokens were found.</p>

  const copy = async (text) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(text)
      setTimeout(() => setCopied(''), 1600)
    } catch {
      setCopied('')
    }
  }
  const none = !tokens.colors.length && !tokens.typography.length && !tokens.spacing.length && !tokens.radii.length && !tokens.shadows.length
  if (none) return <p className="tab-note">This design has no reusable tokens.</p>

  return (
    <div className="tab-body tokens">
      {tokens.colors.length > 0 && (
        <section>
          <h3>Colors ({tokens.colors.length})</h3>
          <div className="token-colors">
            {tokens.colors.map((c) => (
              <button key={`${c.hex}-${c.alpha}`} type="button" className="token-color" onClick={() => copy(c.hex)} aria-label={`Copy color ${c.hex}`}>
                <span style={{ background: c.hex, opacity: c.alpha < 1 ? Math.max(c.alpha, 0.15) : 1 }} />
                <b>{copied === c.hex ? 'Copied' : c.hex}</b>
                <small>
                  {c.alpha < 1 ? `${Math.round(c.alpha * 100)}% · ` : ''}used {c.count}×
                </small>
              </button>
            ))}
          </div>
        </section>
      )}

      {tokens.typography.length > 0 && (
        <section>
          <h3>Text styles ({tokens.typography.length})</h3>
          <ul className="token-type">
            {tokens.typography.map((t, i) => (
              <li key={i}>
                <span className="sample" style={{ fontFamily: `"${t.family}", system-ui, sans-serif`, fontWeight: t.weight || 400, fontSize: Math.min(t.size, 30) }}>
                  Aa
                </span>
                <span className="meta">
                  <b>{t.family}</b>
                  <small>
                    {weightName(t.weight)} · {t.size}px{t.line_height ? ` · line ${t.line_height}` : ''}
                    {t.letter_spacing ? ` · spacing ${t.letter_spacing}px` : ''}
                  </small>
                </span>
                <small className="uses">{t.count}×</small>
              </li>
            ))}
          </ul>
        </section>
      )}

      {tokens.spacing.length > 0 && (
        <section>
          <h3>Spacing ({tokens.spacing.length})</h3>
          <div className="chips">
            {tokens.spacing.map((v) => (
              <span key={v}>{v}px</span>
            ))}
          </div>
        </section>
      )}

      {tokens.radii.length > 0 && (
        <section>
          <h3>Corner radii ({tokens.radii.length})</h3>
          <div className="chips">
            {tokens.radii.map((v) => (
              <span key={v}>{v}px</span>
            ))}
          </div>
        </section>
      )}

      {tokens.shadows.length > 0 && (
        <section>
          <h3>Shadows ({tokens.shadows.length})</h3>
          <ul className="token-shadows">
            {tokens.shadows.map((s, i) => (
              <li key={i}>
                <span className="shadow-sample" style={{ boxShadow: shadowCss(s) }} />
                <span className="meta">
                  <b>{s.type === 'inner' ? 'Inner shadow' : 'Drop shadow'}</b>
                  <small>
                    x {s.x}, y {s.y}, blur {s.blur}, spread {s.spread}
                    {s.hex ? ` · ${s.hex}` : ''}
                  </small>
                </span>
                <small className="uses">{s.count}×</small>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
