import type { ChatMessageVO, ConversationVO, RoundMessageVO, SSEMessageVO } from '../../api'
import type {
  AssistantMessagePart,
  AssistantStreamEvent,
  AssistantThreadMessage,
  PersistedAssistantMessage,
  ReasoningChunk,
  ThreadSummary,
  ToolCallChunk,
  ToolResultChunk,
} from './types'

const SYNTHETIC_USER_SUFFIX = ':user'

interface StreamEventContext {
  threadId: string
  parentMessageId?: string
}

export function toThreadSummary(conversation: ConversationVO): ThreadSummary {
  return {
    threadId: conversation.conversation_id,
    title: conversation.title,
    archived: false,
    createdAt: conversation.created_at,
  }
}

export function toPersistedAssistantMessage(message: ChatMessageVO): PersistedAssistantMessage {
  const { reasoningChunks, toolCallChunks, toolResultChunks } = extractRoundChunks(message)

  return {
    messageId: message.message_id,
    threadId: message.conversation_id,
    parentMessageId: message.parent_message_id,
    query: message.query,
    response: message.response,
    model: message.model,
    createdAt: message.created_at,
    reasoningChunks,
    toolCallChunks,
    toolResultChunks,
  }
}

export function toAssistantThreadMessages(message: ChatMessageVO): AssistantThreadMessage[] {
  const persistedMessage = toPersistedAssistantMessage(message)
  const userMessageId = toSyntheticUserMessageId(message.message_id)
  const assistantParts: AssistantMessagePart[] = []

  if (persistedMessage.response) {
    assistantParts.push({ type: 'text', text: persistedMessage.response })
  }

  assistantParts.push(...toolCallChunksToMessageParts(persistedMessage.toolCallChunks))
  assistantParts.push(...toolResultChunksToMessageParts(persistedMessage.toolResultChunks))

  return [
    {
      messageId: userMessageId,
      threadId: persistedMessage.threadId,
      parentMessageId: persistedMessage.parentMessageId,
      role: 'user',
      parts: [{ type: 'text', text: persistedMessage.query }],
    },
    {
      messageId: persistedMessage.messageId,
      threadId: persistedMessage.threadId,
      // The backend stores one turn with both query and response. The UI splits that
      // into a synthetic user message followed by the assistant reply.
      parentMessageId: userMessageId,
      role: 'assistant',
      parts: assistantParts,
    },
  ]
}

export function toAssistantStreamEvent(
  event: SSEMessageVO,
  context: StreamEventContext,
): AssistantStreamEvent {
  const parentMessageId = context.parentMessageId ?? ''

  if (event.event === 'reasoning') {
    return {
      messageId: event.message_id,
      threadId: context.threadId,
      parentMessageId,
      event: event.event,
      text: event.reasoning_content ?? '',
      reasoningChunks: [toReasoningChunk(event.message_id, parentMessageId, event.reasoning_content)],
      toolCallChunks: [],
      toolResultChunks: [],
    }
  }

  if (event.event === 'tool_call') {
    return {
      messageId: event.message_id,
      threadId: context.threadId,
      parentMessageId,
      event: event.event,
      text: event.tool_call ?? '',
      reasoningChunks: [],
      toolCallChunks: [
        {
          type: 'tool-call',
          messageId: event.message_id,
          parentMessageId,
          toolName: event.tool_call ?? '',
          args: event.tool_arguments ?? '',
        },
      ],
      toolResultChunks: [],
    }
  }

  if (event.event === 'tool_result') {
    return {
      messageId: event.message_id,
      threadId: context.threadId,
      parentMessageId,
      event: event.event,
      text: event.tool_result ?? '',
      reasoningChunks: [],
      toolCallChunks: [],
      toolResultChunks: [
        {
          type: 'tool-result',
          messageId: event.message_id,
          parentMessageId,
          toolName: event.tool_call ?? '',
          result: event.tool_result ?? '',
        },
      ],
    }
  }

  return {
    messageId: event.message_id,
    threadId: context.threadId,
    parentMessageId,
    event: event.event,
    text: event.content ?? '',
    reasoningChunks: [],
    toolCallChunks: [],
    toolResultChunks: [],
  }
}

// query becomes the user message, response becomes the assistant text part,
// rounds[].tool_calls become tool-call parts, and rounds[role=tool] become
// tool-result parts. Reasoning is left as side-channel state unless a later UI
// explicitly decides to promote it into canonical message parts. Fetched history
// currently cannot reconstruct reasoning because ChatMessageVO does not expose it.
export function extractRoundChunks(message: ChatMessageVO): {
  reasoningChunks: ReasoningChunk[]
  toolCallChunks: ToolCallChunk[]
  toolResultChunks: ToolResultChunk[]
} {
  const reasoningChunks: ReasoningChunk[] = []
  const toolCallChunks: ToolCallChunk[] = []
  const toolResultChunks: ToolResultChunk[] = []
  const toolNameById = new Map<string, string>()

  for (const round of message.rounds ?? []) {
    if (round.role === 'assistant' && round.tool_calls?.length) {
      for (const toolCall of round.tool_calls) {
        toolNameById.set(toolCall.id, toolCall.name)
      }
      toolCallChunks.push(...toToolCallChunks(message.message_id, message.parent_message_id, round))
    }
  }

  for (const round of message.rounds ?? []) {
    if (round.role === 'tool') {
      toolResultChunks.push(
        toToolResultChunk(message.message_id, message.parent_message_id, round, toolNameById),
      )
    }
  }

  return { reasoningChunks, toolCallChunks, toolResultChunks }
}

export function toolCallChunksToMessageParts(chunks: ToolCallChunk[]): AssistantMessagePart[] {
  return chunks.map((chunk) => ({
    type: 'tool-call',
    toolCallId: chunk.toolCallId,
    toolName: chunk.toolName,
    args: chunk.args,
  }))
}

export function toolResultChunksToMessageParts(chunks: ToolResultChunk[]): AssistantMessagePart[] {
  return chunks.map((chunk) => ({
    type: 'tool-result',
    toolCallId: chunk.toolCallId,
    toolName: chunk.toolName,
    result: chunk.result,
  }))
}

function toSyntheticUserMessageId(messageId: string): string {
  return `${messageId}${SYNTHETIC_USER_SUFFIX}`
}

function toReasoningChunk(
  messageId: string,
  parentMessageId: string,
  text?: string,
): ReasoningChunk {
  return {
    type: 'reasoning',
    messageId,
    parentMessageId,
    text: text ?? '',
  }
}

function toToolCallChunks(
  messageId: string,
  parentMessageId: string,
  round: RoundMessageVO,
): ToolCallChunk[] {
  return (round.tool_calls ?? []).map((toolCall) => ({
    type: 'tool-call',
    messageId,
    parentMessageId,
    toolCallId: toolCall.id,
    toolName: toolCall.name,
    args: toolCall.arguments,
  }))
}

function toToolResultChunk(
  messageId: string,
  parentMessageId: string,
  round: RoundMessageVO,
  toolNameById: Map<string, string>,
): ToolResultChunk {
  return {
    type: 'tool-result',
    messageId,
    parentMessageId,
    toolCallId: round.tool_id || undefined,
    toolName: round.tool_name || (round.tool_id ? toolNameById.get(round.tool_id) ?? '' : ''),
    result: round.content ?? '',
  }
}
