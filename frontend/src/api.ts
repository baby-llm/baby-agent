export const USER_ID = 'user_001'
const BASE = '/api'

interface APIResponse<T> {
  code: number
  msg: string
  data?: T
}

export interface ConversationVO {
  conversation_id: string
  user_id: string
  title: string
  created_at: number
}

export interface ToolCallVO {
  id: string
  name: string
  arguments: string
}

export interface RoundMessageVO {
  role: 'user' | 'assistant' | 'tool'
  content?: string
  tool_calls?: ToolCallVO[]
  tool_name?: string
  tool_id?: string
}

export interface ChatMessageVO {
  message_id: string
  conversation_id: string
  parent_message_id: string
  query: string
  response: string
  model: string
  created_at: number
  rounds?: RoundMessageVO[]
}

export interface SSEMessageVO {
  message_id: string
  event: 'error' | 'reasoning' | 'content' | 'tool_call' | 'tool_result'
  content?: string
  reasoning_content?: string
  tool_call?: string
  tool_arguments?: string
  tool_result?: string
}

interface StreamThreadRunArgs {
  threadId: string
  query: string
  parentMessageId?: string
  signal?: AbortSignal
  onEvent: (event: SSEMessageVO) => void
  onClose: () => void
  onError?: (error: Error) => void
}

export type ThreadOperation = 'rename' | 'archive' | 'delete'

export interface ThreadOperationUnsupported {
	ok: false
	unsupported: true
	operation: ThreadOperation
  threadId: string
  message: string
}

export type ThreadOperationResult = ThreadOperationUnsupported

export const THREAD_OPERATION_SUPPORT: Record<ThreadOperation, boolean> = {
  rename: true,
  archive: false,
  delete: true,
}

export async function fetchThreads(): Promise<ConversationVO[]> {
  const json = await requestJSON<ConversationVO[]>(`${BASE}/conversation?user_id=${USER_ID}`)
  return json.data ?? []
}

export async function createThread(title = 'New Chat'): Promise<ConversationVO> {
  const json = await requestJSON<ConversationVO>(`${BASE}/conversation`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: USER_ID, title }),
  })
  if (!json.data) throw new Error('Conversation was not returned by the server')
  return json.data
}

export async function renameThread(threadId: string, title: string): Promise<ConversationVO> {
  const json = await requestJSON<ConversationVO>(`${BASE}/conversation/${threadId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
  if (!json.data) throw new Error('Conversation rename did not return updated data')
  return json.data
}

export async function archiveThread(threadId: string): Promise<ThreadOperationResult> {
  return unsupportedThreadOperation('archive', threadId)
}

export async function deleteThread(threadId: string): Promise<void> {
  await requestJSON<{ conversation_id: string }>(`${BASE}/conversation/${threadId}`, {
    method: 'DELETE',
  })
}

export async function fetchThreadMessages(threadId: string): Promise<ChatMessageVO[]> {
  const json = await requestJSON<ChatMessageVO[]>(`${BASE}/conversation/${threadId}/message`)
  return json.data ?? []
}

export function streamThreadRun({
  threadId,
  query,
  parentMessageId,
  signal,
  onEvent,
  onClose,
  onError,
}: StreamThreadRunArgs): () => void {
  const ctrl = new AbortController()
  const cleanup = bindAbortSignal(signal, ctrl)
  let finalized = false

  const finalize = (callback?: () => void) => {
    if (finalized) return
    finalized = true
    cleanup()
    callback?.()
  }

  void readSSEStream({
    threadId,
    query,
    parentMessageId,
    signal: ctrl.signal,
    onEvent,
  })
    .then(() => finalize(onClose))
    .catch((err) => {
      if (isAbortError(err)) {
        finalize()
        return
      }

      const error = normalizeStreamError(err)
      console.error('SSE error:', error)
      finalize(() => {
        onError?.(error)
        onClose()
      })
    })

  return () => {
    finalize()
    ctrl.abort()
  }
}

interface ReadSSEStreamArgs {
  threadId: string
  query: string
  parentMessageId?: string
  signal: AbortSignal
  onEvent: (event: SSEMessageVO) => void
}

// Use the platform stream reader instead of a reconnecting EventSource wrapper.
// A chat response is one POST request: retrying it can create duplicate turns and
// leave the assistant-ui placeholder in a permanent running state.
async function readSSEStream({
  threadId,
  query,
  parentMessageId,
  signal,
  onEvent,
}: ReadSSEStreamArgs): Promise<void> {
  const response = await fetch(`${BASE}/conversation/${threadId}/message`, {
    method: 'POST',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ user_id: USER_ID, query, parent_message_id: parentMessageId ?? '' }),
    signal,
  })

  if (!response.ok) {
    throw new Error(`Chat request failed (${response.status} ${response.statusText})`)
  }
  if (!response.body) {
    throw new Error('Chat request returned no response stream')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    buffer += decoder.decode(value, { stream: !done })

    const events = buffer.split(/\r?\n\r?\n/)
    buffer = events.pop() ?? ''
    for (const rawEvent of events) {
      const data = rawEvent
        .split(/\r?\n/)
        .filter((line) => line.startsWith('data:'))
        .map((line) => line.slice(5).trimStart())
        .join('\n')
      const event = parseSSEMessage(data)
      if (event) onEvent(event)
    }

    if (done) break
  }
}

export const listConversations = fetchThreads
export const createConversation = createThread
export const listMessages = fetchThreadMessages

export function streamMessage(
  conversationId: string,
  query: string,
  onEvent: (event: SSEMessageVO) => void,
  onClose: () => void,
  parentMessageId?: string,
  onError?: (error: Error) => void,
): () => void {
  return streamThreadRun({
    threadId: conversationId,
    query,
    parentMessageId,
    onEvent,
    onClose,
    onError,
  })
}

async function requestJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<APIResponse<T>> {
  const res = await fetch(input, init)
  const json = await res.json() as APIResponse<T>
  if (json.code !== 0) throw new Error(json.msg)
  return json
}

function unsupportedThreadOperation(
  operation: ThreadOperation,
  threadId: string,
): ThreadOperationUnsupported {
  return {
    ok: false,
    unsupported: true,
    operation,
    threadId,
    message: `${operation}Thread is not implemented by the backend yet`,
  }
}

function parseSSEMessage(data: string): SSEMessageVO | null {
  try {
    return JSON.parse(data) as SSEMessageVO
  } catch {
    return null
  }
}

function bindAbortSignal(signal: AbortSignal | undefined, ctrl: AbortController): () => void {
  if (!signal) return () => {}
  if (signal.aborted) {
    ctrl.abort(signal.reason)
    return () => {}
  }

  const abort = () => ctrl.abort(signal.reason)
  signal.addEventListener('abort', abort, { once: true })
  return () => signal.removeEventListener('abort', abort)
}

function isAbortError(err: unknown): err is Error {
  return err instanceof Error && err.name === 'AbortError'
}

function normalizeStreamError(err: unknown): Error {
  if (err instanceof Error) return err
  if (typeof err === 'string') return new Error(err)

  try {
    return new Error(JSON.stringify(err))
  } catch {
    return new Error('Unknown SSE transport error')
  }
}
