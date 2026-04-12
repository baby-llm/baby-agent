import { Send } from 'lucide-react'
import { ComposerPrimitive } from '@assistant-ui/react'

import { Button } from '../ui/button'
import { Textarea } from '../ui/textarea'

export default function AssistantComposer() {
  return (
    <div
      style={{
        borderTop: '1px solid var(--border)',
        padding: '16px 24px 20px',
        background: 'var(--sidebar-bg)',
      }}
    >
      <ComposerPrimitive.Root
        style={{
          margin: '0 auto',
          width: '100%',
          maxWidth: 960,
          display: 'flex',
          gap: 8,
          alignItems: 'flex-end',
        }}
      >
        <ComposerPrimitive.Input
          render={<Textarea />}
          placeholder="Send a message..."
          submitMode="enter"
          style={{
            flex: 1,
            width: '100%',
            minWidth: 0,
            overflowY: 'auto',
          }}
        />
        <ComposerPrimitive.Send asChild>
          <Button size="icon" aria-label="Send message">
            <Send size={16} />
          </Button>
        </ComposerPrimitive.Send>
      </ComposerPrimitive.Root>
    </div>
  )
}
