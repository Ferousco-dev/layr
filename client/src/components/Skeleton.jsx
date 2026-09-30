import './feedback.css'

// Skeleton is a shimmering placeholder block; size it with style or a parent layout.
export default function Skeleton({ className = '', style }) {
  return <span className={`skeleton ${className}`.trim()} style={style} aria-hidden="true" />
}
