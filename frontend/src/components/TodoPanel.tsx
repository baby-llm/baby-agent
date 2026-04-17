import type { PlanningState } from '../api'

interface Props {
  planState: PlanningState | null
}

const STATUS_LABEL: Record<'pending' | 'in_progress' | 'completed', string> = {
  pending: 'Pending',
  in_progress: 'In Progress',
  completed: 'Completed',
}

export default function TodoPanel({ planState }: Props) {
  return (
    <aside style={{
      width: 320,
      borderLeft: '1px solid var(--border)',
      background: 'var(--sidebar-bg)',
      padding: '20px 16px',
      overflowY: 'auto',
      flexShrink: 0,
    }}>
      <div style={{ marginBottom: 16 }}>
        <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--text)', marginBottom: 4 }}>
          Current Plan
        </div>
        <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>
          The agent updates this list with `todo_write`.
        </div>
      </div>

      {!planState || planState.items.length === 0 ? (
        <div style={{
          border: '1px dashed var(--border)',
          borderRadius: 12,
          padding: '14px 12px',
          fontSize: 13,
          lineHeight: 1.6,
          color: 'var(--text-muted)',
          background: 'var(--panel-bg)',
        }}>
          No active todo list yet. Once the task becomes multi-step, the agent can publish a plan here.
        </div>
      ) : (
        <>
          <div style={{ display: 'grid', gap: 10 }}>
            {planState.items.map((item, index) => {
              const active = item.status === 'in_progress'
              const done = item.status === 'completed'
              return (
                <div
                  key={`${planState.revision}-${index}-${item.content}`}
                  style={{
                    border: `1px solid ${active ? 'rgba(124,58,237,0.45)' : 'var(--border)'}`,
                    background: active ? 'rgba(124,58,237,0.12)' : 'var(--panel-bg)',
                    borderRadius: 12,
                    padding: '12px 12px 10px',
                  }}
                >
                  <div style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 8,
                    marginBottom: 8,
                  }}>
                    <span style={{
                      fontSize: 11,
                      letterSpacing: '0.04em',
                      textTransform: 'uppercase',
                      color: active ? 'var(--accent-light)' : done ? '#86efac' : 'var(--text-muted)',
                    }}>
                      {STATUS_LABEL[item.status]}
                    </span>
                    <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                      {index + 1}
                    </span>
                  </div>
                  <div style={{
                    fontSize: 13,
                    lineHeight: 1.6,
                    color: done ? 'var(--text-muted)' : 'var(--text)',
                    textDecoration: done ? 'line-through' : 'none',
                  }}>
                    {item.content}
                  </div>
                </div>
              )
            })}
          </div>

          <div style={{
            marginTop: 14,
            fontSize: 11,
            color: 'var(--text-muted)',
          }}>
            Revision {planState.revision} · Updated in loop {planState.last_updated_loop}
          </div>
        </>
      )}
    </aside>
  )
}
