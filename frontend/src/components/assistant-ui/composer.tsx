import { Send } from 'lucide-react'
import { ComposerPrimitive } from '@assistant-ui/react'

export default function AssistantComposer() {
  return (
    <div
      style={{
        borderTop: '1px solid var(--border)',
        padding: '12px 16px',
        background: 'var(--sidebar-bg)',
      }}
    >
      <ComposerPrimitive.Root
        style={{
          margin: '0 auto',
          display: 'flex',
          gap: 8,
          alignItems: 'flex-end',
        }}
      >
        <ComposerPrimitive.Input
          placeholder="Send a message..."
          submitMode="enter"
          style={{
            flex: 1,
            resize: 'none',
            background: 'var(--panel-bg)',
            border: '1px solid var(--border)',
            borderRadius: 10,
            padding: '10px 14px',
            color: 'var(--text)',
            fontSize: 14,
            outline: 'none',
            lineHeight: 1.5,
            fontFamily: 'inherit',
            overflowY: 'auto',
          }}
        />
        <ComposerPrimitive.Send
          style={{
            background: 'var(--accent)',
            border: 'none',
            borderRadius: 10,
            padding: '10px 14px',
            cursor: 'pointer',
            color: '#fff',
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <Send size={16} />
        </ComposerPrimitive.Send>
      </ComposerPrimitive.Root>
    </div>
  )
}
