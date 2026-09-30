export function FigmaMark() {
  return (
    <svg className="figma" viewBox="0 0 38 57" aria-hidden="true">
      <path fill="#f24e1e" d="M9.5 0h9.5v19H9.5a9.5 9.5 0 010-19" />
      <path fill="#ff7262" d="M19 0h9.5a9.5 9.5 0 010 19H19" />
      <path fill="#a259ff" d="M9.5 19H19v19H9.5a9.5 9.5 0 010-19" />
      <circle fill="#1abcfe" cx="28.5" cy="28.5" r="9.5" />
      <path fill="#0acf83" d="M9.5 38H19v9.5A9.5 9.5 0 119.5 38" />
    </svg>
  )
}

export function ArrowRight() {
  return (
    <svg className="arrow" viewBox="0 0 34 34" fill="none" stroke="currentColor" strokeWidth="3" aria-hidden="true">
      <path d="M3 17h27M19 5l12 12-12 12" />
    </svg>
  )
}

const stroke = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.75, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true }

export const EyeIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" {...stroke}>
    <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7Z" />
    <circle cx="12" cy="12" r="3" />
  </svg>
)
export const EyeOffIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" {...stroke}>
    <path d="M17.9 17.9A10.4 10.4 0 0 1 12 19c-6 0-10-7-10-7a17.6 17.6 0 0 1 4.1-4.9M9.9 5.2A9.6 9.6 0 0 1 12 5c6 0 10 7 10 7a17.7 17.7 0 0 1-2.2 3M3 3l18 18M9.9 9.9a3 3 0 0 0 4.2 4.2" />
  </svg>
)
export const MailIcon = () => (
  <svg viewBox="0 0 24 24" width="20" height="20" {...stroke}>
    <rect x="3" y="5" width="18" height="14" rx="2" />
    <path d="m3 6 9 7 9-7" />
  </svg>
)
export const TrashIcon = () => (
  <svg viewBox="0 0 24 24" width="22" height="22" {...stroke}>
    <path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7" />
  </svg>
)
export const LogoutIcon = () => (
  <svg viewBox="0 0 24 24" width="22" height="22" {...stroke}>
    <path d="M10 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h5M9 12h12m-5-5 5 5-5 5" />
  </svg>
)
export const CheckIcon = () => (
  <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="m5 12 5 5 9-10" />
  </svg>
)
