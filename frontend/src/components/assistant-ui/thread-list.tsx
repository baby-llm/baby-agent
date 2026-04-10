import { MessageSquarePlus } from 'lucide-react'
import {
  ThreadListItemPrimitive,
  ThreadListPrimitive,
  useAuiState,
} from '@assistant-ui/react'

export default function AssistantThreadList() {
  const mainThreadId = useAuiState((s) => s.threads.mainThreadId)

  return (
    <aside
      style={{
        width: 260,
        background: 'var(--sidebar-bg)',
        borderRight: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
        flexShrink: 0,
      }}
    >
      <div
        style={{
          padding: '16px 12px',
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--text)' }}>
          Assistant UI
        </span>
        <ThreadListPrimitive.New
          title="New Chat"
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'var(--text-muted)',
            padding: 4,
            borderRadius: 6,
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <MessageSquarePlus size={18} />
        </ThreadListPrimitive.New>
      </div>

      <ThreadListPrimitive.Root
        style={{ flex: 1, overflowY: 'auto', padding: '8px 6px' }}
      >
        <ThreadListPrimitive.Items>
          {({ threadListItem }) => {
            const active = threadListItem.id === mainThreadId
            return (
              <ThreadListItemPrimitive.Root
                key={threadListItem.id}
                style={{
                  marginBottom: 4,
                }}
              >
                <ThreadListItemPrimitive.Trigger
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    background: active ? 'rgba(124,58,237,0.15)' : 'transparent',
                    border: active
                      ? '1px solid rgba(124,58,237,0.4)'
                      : '1px solid transparent',
                    borderRadius: 8,
                    padding: '8px 10px',
                    cursor: 'pointer',
                    color: active ? 'var(--accent-light)' : 'var(--text)',
                    fontSize: 13,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  <ThreadListItemPrimitive.Title />
                </ThreadListItemPrimitive.Trigger>
              </ThreadListItemPrimitive.Root>
            )
          }}
        </ThreadListPrimitive.Items>
      </ThreadListPrimitive.Root>
    </aside>
  )
}
