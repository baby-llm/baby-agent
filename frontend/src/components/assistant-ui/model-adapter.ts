import type {
  ChatModelAdapter,
  ChatModelRunResult,
  ThreadAssistantMessagePart,
  ThreadMessage,
  ToolCallMessagePart,
} from '@assistant-ui/react'
import type { ReadonlyJSONObject } from 'assistant-stream/utils'

import { streamThreadRun, type SSEMessageVO } from '../../api'

interface TransportErrorItem {
  type: 'transport-error'
  error: Error
}

type StreamQueueItem = SSEMessageVO | TransportErrorItem | typeof STREAM_DONE

const STREAM_DONE = Symbol('stream-done')
const DEFAULT_TOOL_NAME = 'tool'

interface BackendMetadata {
  backendMessageId?: string
}

interface RunState {
  backendMessageId?: string
  parts: ThreadAssistantMessagePart[]
  textIndex: number | null
  reasoningIndex: number | null
  toolCallSequence: number
}

class AsyncQueue<T> {
  private items: T[] = []
  private resolvers: Array<(value: T) => void> = []
  private closed = false

  push(item: T): void {
    if (this.closed) return
    const resolve = this.resolvers.shift()
    if (resolve) {
      resolve(item)
      return
    }
    this.items.push(item)
  }

  close(finalItem: T): void {
    if (this.closed) return
    this.closed = true
    if (this.resolvers.length > 0) {
      for (const resolve of this.resolvers.splice(0)) resolve(finalItem)
      return
    }
    this.items.push(finalItem)
  }

  async next(): Promise<T> {
    const item = this.items.shift()
    if (item !== undefined) return item
    return new Promise<T>((resolve) => this.resolvers.push(resolve))
  }
}

export const babyAgentChatModelAdapter: ChatModelAdapter = {
  async *run(options) {
    const threadId = options.unstable_threadId
    if (!threadId) {
      throw new Error('assistant-ui did not provide an active thread id for the local runtime')
    }

    const query = getLatestUserQuery(options.messages)
    if (!query) {
      throw new Error('Unable to derive the latest user message text for the backend request')
    }

    const queue = new AsyncQueue<StreamQueueItem>()
    const state: RunState = {
      backendMessageId: undefined,
      parts: [],
      textIndex: null,
      reasoningIndex: null,
      toolCallSequence: 0,
    }

    const stop = streamThreadRun({
      threadId,
      query,
      parentMessageId: getLatestBackendMessageId(options.messages),
      signal: options.abortSignal,
      onEvent: (event) => queue.push(event),
      onError: (error) => {
        queue.push({ type: 'transport-error', error })
        queue.close(STREAM_DONE)
      },
      onClose: () => queue.close(STREAM_DONE),
    })

    const abort = () => {
      stop()
      queue.close(STREAM_DONE)
    }

    options.abortSignal.addEventListener('abort', abort, { once: true })

    try {
      while (true) {
        const item = await queue.next()
        if (item === STREAM_DONE) break
        if (isTransportErrorItem(item)) throw item.error

        const update = applySSEEvent(state, item)
        if (update) yield update
      }
    } finally {
      stop()
      options.abortSignal.removeEventListener('abort', abort)
    }
  },
}

function getLatestUserQuery(messages: readonly ThreadMessage[]): string {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message.role !== 'user') continue
    return extractTextContent(message).trim()
  }
  return ''
}

function getLatestBackendMessageId(messages: readonly ThreadMessage[]): string | undefined {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message.role !== 'assistant') continue
    const backendMessageId = (message.metadata.custom as BackendMetadata | undefined)?.backendMessageId
    if (backendMessageId) return backendMessageId
  }
  return undefined
}

function extractTextContent(message: ThreadMessage): string {
  return message.content
    .filter((part) => part.type === 'text')
    .map((part) => part.text)
    .join('')
}

function applySSEEvent(state: RunState, event: SSEMessageVO): ChatModelRunResult | null {
  state.backendMessageId = event.message_id

  switch (event.event) {
    case 'content':
      appendTextPart(state, event.content ?? '')
      break
    case 'reasoning':
      appendReasoningPart(state, event.reasoning_content ?? '')
      break
    case 'tool_call':
      appendToolCallPart(state, event)
      break
    case 'tool_result':
      appendToolResult(state, event)
      break
    case 'error':
      throw new Error(event.content || 'The backend returned an error event while streaming the assistant response')
    default: {
      const exhaustiveCheck: never = event.event
      throw new Error(`Unsupported SSE event: ${exhaustiveCheck}`)
    }
  }

  return {
    content: cloneAssistantParts(state.parts),
    metadata: {
      custom: {
        backendMessageId: state.backendMessageId,
      },
    },
  }
}

function isTransportErrorItem(item: StreamQueueItem): item is TransportErrorItem {
  return item !== STREAM_DONE && 'type' in item && item.type === 'transport-error'
}

function appendTextPart(state: RunState, delta: string): void {
  if (!delta) return

  if (state.textIndex === null) {
    state.textIndex = state.parts.push({ type: 'text', text: delta }) - 1
    return
  }

  const current = state.parts[state.textIndex]
  if (current?.type !== 'text') return

  state.parts[state.textIndex] = {
    ...current,
    text: current.text + delta,
  }
}

function appendReasoningPart(state: RunState, delta: string): void {
  if (!delta) return

  if (state.reasoningIndex === null) {
    state.reasoningIndex = state.parts.push({ type: 'reasoning', text: delta }) - 1
    return
  }

  const current = state.parts[state.reasoningIndex]
  if (current?.type !== 'reasoning') return

  state.parts[state.reasoningIndex] = {
    ...current,
    text: current.text + delta,
  }
}

function appendToolCallPart(state: RunState, event: SSEMessageVO): void {
  const toolName = event.tool_call || DEFAULT_TOOL_NAME
  const argsText = event.tool_arguments ?? ''
  state.toolCallSequence += 1
  state.parts.push({
    type: 'tool-call',
    toolCallId: `${event.message_id}:${state.toolCallSequence}`,
    toolName,
    args: parseToolArgs(argsText),
    argsText,
  })
}

function appendToolResult(state: RunState, event: SSEMessageVO): void {
  const toolName = event.tool_call || DEFAULT_TOOL_NAME
  const result = parseToolResult(event.tool_result)
  const targetIndex = findPendingToolCallIndex(state.parts, toolName)

  if (targetIndex === -1) {
    state.toolCallSequence += 1
    state.parts.push({
      type: 'tool-call',
      toolCallId: `${event.message_id}:${state.toolCallSequence}`,
      toolName,
      args: {},
      argsText: '',
      result,
    })
    return
  }

  const current = state.parts[targetIndex]
  if (current?.type !== 'tool-call') return

  state.parts[targetIndex] = {
    ...current,
    result,
  }
}

function findPendingToolCallIndex(parts: readonly ThreadAssistantMessagePart[], toolName: string): number {
  for (let index = parts.length - 1; index >= 0; index -= 1) {
    const part = parts[index]
    if (part.type !== 'tool-call') continue
    if (part.toolName !== toolName) continue
    if (part.result !== undefined) continue
    return index
  }
  return -1
}

function parseToolArgs(argsText: string): ReadonlyJSONObject {
  const parsed = parseJSON(argsText)
  if (isPlainObject(parsed)) return parsed as ReadonlyJSONObject
  if (!argsText.trim()) return {}
  return { raw: (parsed ?? argsText) as string }
}

function parseToolResult(resultText: string | undefined): unknown {
  if (!resultText) return ''
  return parseJSON(resultText) ?? resultText
}

function parseJSON(value: string): unknown {
  if (!value.trim()) return undefined
  try {
    return JSON.parse(value)
  } catch {
    return undefined
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function cloneAssistantParts(parts: readonly ThreadAssistantMessagePart[]): ThreadAssistantMessagePart[] {
  return parts.map((part) => {
    if (part.type === 'tool-call') {
      const toolPart: ToolCallMessagePart = {
        ...part,
        args: { ...part.args },
      }
      return toolPart
    }
    return { ...part }
  })
}
