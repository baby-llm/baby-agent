import { useEffect, useState } from 'react'
import type {
  ExportedMessageRepository,
  ThreadAssistantMessage,
  ThreadAssistantMessagePart,
  ThreadUserMessage,
  ToolCallMessagePart,
} from '@assistant-ui/react'
import { ThreadPrimitive, useAui, useAuiState } from '@assistant-ui/react'
import type { ReadonlyJSONObject, ReadonlyJSONValue } from 'assistant-stream/utils'

import { fetchThreadMessages, type ChatMessageVO, type RoundMessageVO } from '../../api'
import AssistantComposer from './composer'
import AssistantThreadMessage from './message'

export default function AssistantThread() {
  const aui = useAui()
  const remoteId = useAuiState((s) => {
    const activeThreadId = s.threads.mainThreadId
    return s.threads.threadItems.find((item) => item.id === activeThreadId)?.remoteId
  })
  const messageCount = useAuiState((s) => s.thread.messages.length)
  const isLoading = useAuiState((s) => s.thread.isLoading)
  const isRunning = useAuiState((s) => s.thread.isRunning)
  const [hydratedRemoteId, setHydratedRemoteId] = useState<string | null>(null)
  const [hydrationError, setHydrationError] = useState<string | null>(null)

  const needsHydration = Boolean(
    remoteId && messageCount === 0 && hydratedRemoteId !== remoteId && !isRunning,
  )

  useEffect(() => {
    setHydrationError(null)
  }, [remoteId])

  useEffect(() => {
    if (!remoteId) {
      setHydratedRemoteId(null)
      return
    }
    if (!needsHydration) return

    let cancelled = false
    setHydrationError(null)

    void fetchThreadMessages(remoteId)
      .then((history) => {
        if (cancelled) return
        aui.thread().import(buildHistoryRepository(history))
        setHydratedRemoteId(remoteId)
      })
      .catch((error) => {
        if (cancelled) return
        console.error('Failed to hydrate thread history:', error)
        setHydrationError(error instanceof Error ? error.message : 'Unknown history error')
      })

    return () => {
      cancelled = true
    }
  }, [aui, needsHydration, remoteId])

  return (
    <ThreadPrimitive.Root
      style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        background: 'var(--bg)',
        overflow: 'hidden',
      }}
    >
      <ThreadPrimitive.Viewport
        style={{
          flex: 1,
          overflowY: 'auto',
          padding: '24px 24px 0',
        }}
      >
        {messageCount === 0 && !isLoading && !isRunning ? (
          <div
            style={{
              textAlign: 'center',
              color: hydrationError ? '#fca5a5' : 'var(--text-muted)',
              marginTop: 80,
              fontSize: 14,
            }}
          >
            {hydrationError
              ? 'Failed to load conversation history.'
              : needsHydration
                ? 'Loading conversation...'
                : 'Start a conversation...'}
          </div>
        ) : null}

        <ThreadPrimitive.Messages>
          {() => <AssistantThreadMessage />}
        </ThreadPrimitive.Messages>
      </ThreadPrimitive.Viewport>

      {!needsHydration && !hydrationError ? <AssistantComposer /> : null}
    </ThreadPrimitive.Root>
  )
}

function buildHistoryRepository(history: ChatMessageVO[]): ExportedMessageRepository {
  const messages: ExportedMessageRepository['messages'] = []

  for (const item of history) {
    const userMessageId = toUserMessageId(item.message_id)
    const userMessage: ThreadUserMessage = {
      id: userMessageId,
      role: 'user',
      createdAt: toDate(item.created_at),
      content: [{ type: 'text', text: item.query }],
      attachments: [],
      metadata: {
        custom: {},
      },
    }

    const assistantMessage: ThreadAssistantMessage = {
      id: item.message_id,
      role: 'assistant',
      createdAt: toDate(item.created_at),
      content: buildAssistantParts(item),
      status: { type: 'complete', reason: 'stop' },
      metadata: {
        unstable_state: null,
        unstable_annotations: [],
        unstable_data: [],
        steps: [],
        custom: {
          backendMessageId: item.message_id,
        },
      },
    }

    messages.push({
      parentId: item.parent_message_id || null,
      message: userMessage,
    })
    messages.push({
      parentId: userMessageId,
      message: assistantMessage,
    })
  }

  return {
    headId: history.length > 0 ? history[history.length - 1]?.message_id ?? null : null,
    messages,
  }
}

function buildAssistantParts(item: ChatMessageVO): ThreadAssistantMessagePart[] {
  const parts: ThreadAssistantMessagePart[] = [...buildToolParts(item.rounds ?? [])]

  if (item.response) {
    parts.push({ type: 'text', text: item.response })
  }

  return parts
}

function buildToolParts(rounds: RoundMessageVO[]): ToolCallMessagePart[] {
  const toolParts: ToolCallMessagePart[] = []
  const toolIndexes = new Map<string, number>()

  for (const round of rounds) {
    if (round.role !== 'assistant' || !round.tool_calls?.length) continue

    for (const toolCall of round.tool_calls) {
      toolIndexes.set(toolCall.id, toolParts.length)
      toolParts.push({
        type: 'tool-call',
        toolCallId: toolCall.id,
        toolName: toolCall.name,
        args: parseToolArgs(toolCall.arguments),
        argsText: toolCall.arguments,
      })
    }
  }

  for (const round of rounds) {
    if (round.role !== 'tool' || !round.tool_id) continue
    const index = toolIndexes.get(round.tool_id)
    if (index === undefined) continue

    const current = toolParts[index]
    if (!current) continue

    toolParts[index] = {
      ...current,
      result: parseJSON(round.content ?? '') ?? round.content,
    }
  }

  return toolParts
}

function parseToolArgs(argsText: string): ReadonlyJSONObject {
  const parsed = parseJSON(argsText)
  if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
    return parsed as ReadonlyJSONObject
  }
  if (!argsText.trim()) return {}
  return { raw: (parsed ?? argsText) as ReadonlyJSONValue }
}

function parseJSON(value: string): unknown {
  if (!value.trim()) return undefined
  try {
    return JSON.parse(value)
  } catch {
    return undefined
  }
}

function toUserMessageId(messageId: string): string {
  return `${messageId}:user`
}

function toDate(timestampSeconds: number): Date {
  return new Date(timestampSeconds * 1000)
}
