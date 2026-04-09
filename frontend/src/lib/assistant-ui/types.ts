export interface ThreadSummary {
  threadId: string
  title: string
  archived: boolean
  createdAt: number
}

export interface ReasoningChunk {
  type: 'reasoning'
  messageId: string
  parentMessageId: string
  text: string
}

export interface ToolCallChunk {
  type: 'tool-call'
  messageId: string
  parentMessageId: string
  toolCallId?: string
  toolName: string
  args: string
}

export interface ToolResultChunk {
  type: 'tool-result'
  messageId: string
  parentMessageId: string
  toolCallId?: string
  toolName: string
  result: string
}

export type AssistantMessagePart =
  | { type: 'text'; text: string }
  | { type: 'tool-call'; toolCallId?: string; toolName: string; args: string }
  | { type: 'tool-result'; toolCallId?: string; toolName: string; result: string }

export interface AssistantThreadMessage {
  messageId: string
  threadId: string
  parentMessageId: string
  role: 'user' | 'assistant'
  parts: AssistantMessagePart[]
}

export interface PersistedAssistantMessage {
  messageId: string
  threadId: string
  parentMessageId: string
  query: string
  response: string
  model: string
  createdAt: number
  // History fetches do not include reasoning today, so persisted messages expose
  // an explicit empty array until the backend adds reasoning to ChatMessageVO.
  reasoningChunks: ReasoningChunk[]
  toolCallChunks: ToolCallChunk[]
  toolResultChunks: ToolResultChunk[]
}

export interface AssistantStreamEvent {
  messageId: string
  threadId: string
  parentMessageId: string
  event: 'error' | 'reasoning' | 'content' | 'tool_call' | 'tool_result'
  text: string
  reasoningChunks: ReasoningChunk[]
  toolCallChunks: ToolCallChunk[]
  toolResultChunks: ToolResultChunk[]
}
