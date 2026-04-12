import { useState } from 'react'
import type { ToolCallMessagePartProps } from '@assistant-ui/react'

export default function ToolCallCard({ toolName, argsText, result }: ToolCallMessagePartProps) {
  const [open, setOpen] = useState(false)

  return (
    <div
      style={{
        background: 'var(--tool-bg)',
        border: '1px solid var(--border)',
        borderRadius: 8,
        marginBottom: 8,
        fontSize: 12,
        fontFamily: 'monospace',
        lineHeight: 1.6,
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
          padding: '8px 10px',
          color: '#60a5fa',
          display: 'flex',
          alignItems: 'center',
          gap: 6,
        }}
      >
        <span>{open ? '▼' : '▶'}</span>
        <span>{toolName}</span>
        {result !== undefined ? (
          <span style={{ color: '#4ade80', marginLeft: 'auto' }}>done</span>
        ) : null}
      </button>

      {open ? (
        <div style={{ borderTop: '1px solid var(--border)' }}>
          {argsText ? (
            <pre
              style={{
                margin: 0,
                padding: '8px 10px',
                color: 'var(--text-muted)',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}
            >
              {argsText}
            </pre>
          ) : null}
          {result !== undefined ? (
            <pre
              style={{
                margin: 0,
                padding: '8px 10px',
                color: '#4ade80',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                borderTop: argsText ? '1px solid var(--border)' : undefined,
              }}
            >
              {formatToolResult(result)}
            </pre>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function formatToolResult(result: unknown): string {
  if (typeof result === 'string') return result
  try {
    return JSON.stringify(result, null, 2)
  } catch {
    return String(result)
  }
}
