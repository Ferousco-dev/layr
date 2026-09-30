import openai from '../assets/providers/openai.webp'

// ProviderIcon draws each brand mark at one size; replace with the official assets before launch.
export default function ProviderIcon({ id, size = 51 }) {
  if (id === 'anthropic') {
    const rays = Array.from({ length: 12 }, (_, i) => {
      const a = (i * Math.PI) / 6
      const r2 = i % 2 ? 11 : 15
      return <line key={i} x1={25.5 + Math.cos(a) * 5} y1={25.5 + Math.sin(a) * 5} x2={25.5 + Math.cos(a) * r2} y2={25.5 + Math.sin(a) * r2} />
    })
    return (
      <svg viewBox="0 0 51 51" width={size} height={size} aria-hidden="true">
        <rect width="51" height="51" rx="12" fill="#e2735a" />
        <g stroke="#fff" strokeWidth="3" strokeLinecap="round">
          {rays}
        </g>
      </svg>
    )
  }
  if (id === 'xai') {
    return (
      <svg viewBox="0 0 51 51" width={size} height={size} aria-hidden="true">
        <rect width="51" height="51" rx="12" fill="#0b0b0f" />
        <g stroke="#fff" strokeWidth="5.5">
          <line x1="16" y1="15" x2="35" y2="36" />
          <line x1="35" y1="15" x2="16" y2="36" />
        </g>
      </svg>
    )
  }
  if (id === 'github') {
    return (
      <svg viewBox="0 0 16 16" width={size} height={size} aria-hidden="true">
        <path
          fill="#0f172a"
          d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"
        />
      </svg>
    )
  }
  return <img src={openai} alt="" width={size} height={size} style={{ objectFit: 'contain' }} />
}
