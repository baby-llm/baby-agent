import type {
  ReasoningMessagePartProps,
  ToolCallMessagePartProps,
} from '@assistant-ui/react'
import { MessagePrimitive, useAuiState } from '@assistant-ui/react'

export default function AssistantThreadMessage() {
  const role = useAuiState((s) => s.message.role)
  const isUser = role === 'user'

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: isUser ? 'flex-end' : 'flex-start',
        marginBottom: 16,
      }}
    >
      <MessagePrimitive.Root
        style={{
          maxWidth: '78%',
          background: isUser ? 'var(--user-bubble)' : 'var(--assistant-bubble)',
          border: '1px solid var(--border)',
          borderRadius: isUser ? '12px 12px 2px 12px' : '2px 12px 12px 12px',
          padding: '10px 14px',
          color: 'var(--text)',
          fontSize: 14,
          lineHeight: 1.7,
          whiteSpace: 'pre-wrap',
        }}
      >
        <MessagePrimitive.Parts
          components={{
            Reasoning: ReasoningFallback,
            tools: {
              Fallback: ToolCallFallback,
            },
          }}
        />
      </MessagePrimitive.Root>
    </div>
  )
}

function ReasoningFallback({ text }: ReasoningMessagePartProps) {
  return (
    <pre
      style={{
        margin: '0 0 8px',
        padding: '8px 10px',
        borderRadius: 8,
        background: 'var(--reasoning-bg)',
        color: 'var(--text-muted)',
        fontSize: 12,
        lineHeight: 1.5,
        whiteSpace: 'pre-wrap',
        overflowX: 'auto',
      }}
    >
      {text}
    </pre>
  )
}

function ToolCallFallback({ toolName, argsText, result }: ToolCallMessagePartProps) {
  return (
    <div
      style={{
        marginBottom: 8,
        padding: '8px 10px',
        borderRadius: 8,
        background: 'var(--tool-bg)',
        border: '1px solid var(--border)',
        fontFamily: 'monospace',
        fontSize: 12,
        lineHeight: 1.6,
      }}
    >
      <div style={{ color: '#60a5fa', marginBottom: argsText ? 6 : 0 }}>
        {toolName}
      </div>
      {argsText ? (
        <pre
          style={{
            margin: 0,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            color: 'var(--text-muted)',
          }}
        >
          {argsText}
        </pre>
      ) : null}
      {result !== undefined ? (
        <pre
          style={{
            margin: argsText ? '6px 0 0' : '0',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            color: '#4ade80',
          }}
        >
          {formatToolResult(result)}
        </pre>
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
