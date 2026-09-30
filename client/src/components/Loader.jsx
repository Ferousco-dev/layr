import './feedback.css'

// Loader is the Layr spinner; size is in pixels and everything inside it scales together.
export default function Loader({ size = 70, label }) {
  return <span className="loader" style={{ '--size': `${size}px` }} role={label ? 'status' : undefined} aria-label={label} aria-hidden={label ? undefined : 'true'} />
}
