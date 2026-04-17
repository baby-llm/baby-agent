import { useState, useEffect, useRef } from 'react'
import { Send } from 'lucide-react'
import {
  listMessages,
  createConversation,
  streamMessage,
  type ChatMessageVO,
  type ConversationVO,
  type PlanningState,
  type SSEMessageVO,
} from '../api'
import MessageBubble, { ReasoningBlock } from './MessageBubble'
import TodoPanel from './TodoPanel'

interface StreamingTurn {
  query: string
  reasoning: string
  content: string
  toolEvents: SSEMessageVO[]
  done: boolean
}

interface Props {
  conversationId: string | null
  onConversationCreated?: (conv: ConversationVO) => void
}

export default function ChatPanel({ conversationId, onConversationCreated }: Props) {
  const [history, setHistory] = useState<ChatMessageVO[]>([])
  const [planState, setPlanState] = useState<PlanningState | null>(null)
  const [streaming, setStreaming] = useState<StreamingTurn | null>(null)
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)
  const lastMessageIdRef = useRef<string | undefined>(undefined)
  const convIdRef = useRef<string | null>(conversationId)
  // message_id -> reasoning text, persisted after stream ends
  const reasoningCache = useRef<Map<string, string>>(new Map())
  // message_id -> tool events, persisted after stream ends
  const toolEventsCache = useRef<Map<string, SSEMessageVO[]>>(new Map())

  useEffect(() => {
    convIdRef.current = conversationId
    lastMessageIdRef.current = undefined
    setHistory([])
    setPlanState(null)
    if (!conversationId) return
    listMessages(conversationId)
      .then((msgs) => {
        setHistory(msgs)
        setPlanState(findLatestPlanState(msgs))
        if (msgs.length > 0) lastMessageIdRef.current = msgs[msgs.length - 1].message_id
      })
      .catch(console.error)
  }, [conversationId])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [history, streaming])

  const send = async () => {
    const query = input.trim()
    if (!query || loading) return
    setInput('')
    setLoading(true)

    // Lazily create the conversation on first message
    let activeConvId = convIdRef.current
    if (!activeConvId) {
      const title = query.slice(0, 20)
      const conv = await createConversation(title).catch((e) => {
        console.error(e)
        setLoading(false)
        return null
      })
      if (!conv) return
      activeConvId = conv.conversation_id
      convIdRef.current = activeConvId
      onConversationCreated?.(conv)
    }

    const parentMessageId = lastMessageIdRef.current
    const pollConvId = activeConvId
    const turn: StreamingTurn = { query, reasoning: '', content: '', toolEvents: [], done: false }
    setStreaming(turn)

    streamMessage(activeConvId, query, (e) => {
      if (e.message_id) {
        lastMessageIdRef.current = e.message_id
        // accumulate reasoning into cache keyed by message_id
        if (e.event === 'reasoning' && e.reasoning_content) {
          const prev = reasoningCache.current.get(e.message_id) ?? ''
          reasoningCache.current.set(e.message_id, prev + e.reasoning_content)
        }
      }
      if (e.event === 'todo_snapshot' && e.plan_state) {
        setPlanState(e.plan_state)
      }
      setStreaming((prev) => {
        if (!prev) return prev
        const next = { ...prev }
        if (e.event === 'reasoning' && e.reasoning_content) {
          next.reasoning += e.reasoning_content
        } else if (e.event === 'content' && e.content) {
          next.content += e.content
        } else if (e.event === 'tool_call' || e.event === 'tool_result') {
          next.toolEvents = [...next.toolEvents, e]
        } else if (e.event === 'error') {
          next.content += `\n\n⚠️ Error: ${e.content ?? 'unknown'}`
          next.done = true
        }
        return next
      })
    }, async () => {
      // SSE stream closed — fetch history exactly once
      const msgs = await listMessages(pollConvId).catch(() => null)
      if (msgs) {
        setHistory(msgs)
        setPlanState(findLatestPlanState(msgs))
      }
      setStreaming(null)
      setLoading(false)
    }, parentMessageId)
  }

  const handleKey = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  return (
    <div style={{
      height: '100%',
      display: 'flex',
      background: 'var(--bg)',
    }}>
      <div style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
      }}>
        {/* Messages */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '24px 0' }}>
          <div style={{ padding: '0 24px' }}>
            {history.length === 0 && !streaming && (
              <div style={{
                textAlign: 'center',
                color: 'var(--text-muted)',
                marginTop: 80,
                fontSize: 14,
              }}>
                Start a conversation…
              </div>
            )}
            {history.map((msg) => (
              <MessageBubble
                key={msg.message_id}
                msg={msg}
                reasoning={reasoningCache.current.get(msg.message_id)}
              />
            ))}
            {streaming && <StreamingBubble turn={streaming} />}
            <div ref={bottomRef} />
          </div>
        </div>

        {/* Input */}
        <div style={{
          borderTop: '1px solid var(--border)',
          padding: '12px 16px',
          background: 'var(--sidebar-bg)',
        }}>
          <div style={{
            margin: '0 auto',
            display: 'flex',
            gap: 8,
            alignItems: 'flex-end',
          }}>
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKey}
              placeholder="Send a message… (Enter to send, Shift+Enter for newline)"
              rows={1}
              disabled={loading}
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
                maxHeight: 160,
                overflowY: 'auto',
              }}
            />
            <button
              onClick={send}
              disabled={loading || !input.trim()}
              style={{
                background: loading || !input.trim() ? 'var(--border)' : 'var(--accent)',
                border: 'none',
                borderRadius: 10,
                padding: '10px 14px',
                cursor: loading || !input.trim() ? 'not-allowed' : 'pointer',
                color: '#fff',
                display: 'flex',
                alignItems: 'center',
                transition: 'background 0.15s',
              }}
            >
              <Send size={16} />
            </button>
          </div>
        </div>
      </div>

      <TodoPanel planState={planState} />
    </div>
  )
}

function findLatestPlanState(messages: ChatMessageVO[]): PlanningState | null {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const state = messages[index]?.plan_state
    if (state) return state
  }
  return null
}

function StreamingBubble({ turn }: { turn: StreamingTurn }) {
  return (
    <div style={{ marginBottom: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 12 }}>
        <div style={{
          background: 'var(--user-bubble)',
          borderRadius: '12px 12px 2px 12px',
          padding: '10px 14px',
          maxWidth: '75%',
          fontSize: 14,
          lineHeight: 1.6,
          color: 'var(--text)',
          border: '1px solid var(--border)',
        }}>
          {turn.query}
        </div>
      </div>
      <div style={{ display: 'flex', gap: 10 }}>
        <div style={{
          width: 32, height: 32,
          borderRadius: '50%',
          background: 'var(--accent)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 14, flexShrink: 0,
        }}>🤖</div>
        <div style={{ flex: 1 }}>
          {turn.reasoning && <ReasoningBlock text={turn.reasoning} />}          {turn.toolEvents.map((e, i) => <ToolEvent key={i} event={e} />)}
          {turn.content && (
            <div style={{
              background: 'var(--assistant-bubble)',
              borderRadius: '2px 12px 12px 12px',
              padding: '10px 14px',
              fontSize: 14,
              lineHeight: 1.7,
              color: 'var(--text)',
              whiteSpace: 'pre-wrap',
              border: '1px solid var(--border)',
            }}>
              {turn.content}
              {!turn.done && <span className="cursor-blink">▋</span>}
            </div>
          )}
          {!turn.content && !turn.done && (
            <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>
              <ThinkingDots />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function ToolEvent({ event }: { event: SSEMessageVO }) {
  const [open, setOpen] = useState(false)
  const isCall = event.event === 'tool_call'
  return (
    <div style={{
      background: 'var(--tool-bg)',
      border: '1px solid var(--border)',
      borderRadius: 8,
      marginBottom: 6,
      fontSize: 12,
      fontFamily: 'monospace',
    }}>
      <button
        onClick={() => setOpen((o) => !o)}
        style={{
          width: '100%',
          textAlign: 'left',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          padding: '6px 10px',
          color: isCall ? '#60a5fa' : '#4ade80',
          display: 'flex',
          alignItems: 'center',
          gap: 6,
        }}
      >
        <span>{open ? '▼' : '▶'}</span>
        <span>{isCall ? `⚙ ${event.tool_call}` : `✓ result`}</span>
      </button>
      {open && (
        <div style={{
          padding: '6px 10px 8px',
          color: 'var(--text-muted)',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
          borderTop: '1px solid var(--border)',
        }}>
          {isCall ? event.tool_arguments : event.tool_result}
        </div>
      )}
    </div>
  )
}

function ThinkingDots() {
  return <span style={{ letterSpacing: 2 }}>● ● ●</span>
}
