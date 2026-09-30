export default function Toast({ toast }) {
  if (!toast) return null
  return (
    <div className="toast" role="status">
      <span>{toast.text}</span>
      {toast.undo && (
        <button type="button" onClick={toast.undo}>
          Undo
        </button>
      )}
    </div>
  )
}
