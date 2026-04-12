import { useState } from 'react'
import type { ReasoningMessagePartProps } from '@assistant-ui/react'

export default function ReasoningPanel({ text }: ReasoningMessagePartProps) {
  const [open, setOpen] = useState(false)

  return (
    <div
      style={{
        background: 'var(--reasoning-bg)',
        border: '1px solid var(--border)',
        borderRadius: 8,
        marginBottom: 8,
        fontSize: 13,
      }}
    >
      <button
        onClick={() => setOpen((current) => !current)}
        style={{
          width: '100%',
          textAlign: 'left',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          padding: '8px 12px',
          color: 'var(--text-muted)',
          display: 'flex',
          alignItems: 'center',
          gap: 6,
        }}
      >
        <span>{open ? '▼' : '▶'}</span>
        <span>Thinking</span>
      </button>

      {open ? (
        <pre
          style={{
            margin: 0,
            padding: '8px 12px 10px',
            color: 'var(--text-muted)',
            whiteSpace: 'pre-wrap',
            lineHeight: 1.6,
            borderTop: '1px solid var(--border)',
            overflowX: 'auto',
          }}
        >
          {text}
        </pre>
      ) : null}
    </div>
  )
}
