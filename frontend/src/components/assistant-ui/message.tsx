import { MessagePrimitive, useAuiState } from '@assistant-ui/react'

import ReasoningPanel from './reasoning-panel'
import ToolCallCard from './tool-call-card'

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
            Reasoning: ReasoningPanel,
            tools: {
              Fallback: ToolCallCard,
            },
          }}
        />
      </MessagePrimitive.Root>
    </div>
  )
}
