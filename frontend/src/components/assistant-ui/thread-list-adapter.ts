import type { RemoteThreadListAdapter, ThreadMessage } from '@assistant-ui/react'
import { createAssistantStream } from 'assistant-stream'

import {
  THREAD_OPERATION_SUPPORT,
  archiveThread,
  createThread,
  deleteThread,
  fetchThreads,
  renameThread,
} from '../../api'

export const babyAgentThreadListAdapter: RemoteThreadListAdapter = {
  async list() {
    const conversations = await fetchThreads()
    return {
      threads: conversations.map((conversation) => ({
        status: 'regular' as const,
        remoteId: conversation.conversation_id,
        title: conversation.title,
      })),
    }
  },

  async initialize(localId) {
    const conversation = await createThread()
    return {
      remoteId: conversation.conversation_id,
      externalId: localId,
    }
  },

  async fetch(remoteId) {
    const conversations = await fetchThreads()
    const conversation = conversations.find((entry) => entry.conversation_id === remoteId)
    if (!conversation) {
      throw new Error(`Conversation ${remoteId} was not found on the backend`)
    }

    return {
      status: 'regular' as const,
      remoteId: conversation.conversation_id,
      title: conversation.title,
    }
  },

  async rename(remoteId, newTitle) {
    if (!THREAD_OPERATION_SUPPORT.rename) {
      assertThreadOperationSupported(
        'renameThread is not implemented by the backend yet',
        'rename',
        remoteId,
        false,
      )
    }
    await renameThread(remoteId, newTitle)
  },

  async archive(remoteId) {
    const result = await archiveThread(remoteId)
    assertThreadOperationSupported(result.message, 'archive', remoteId, THREAD_OPERATION_SUPPORT.archive)
  },

  async unarchive(remoteId) {
    assertThreadOperationSupported(
      'unarchiveThread is not implemented by the backend yet',
      'unarchive',
      remoteId,
      false,
    )
  },

  async delete(remoteId) {
    if (!THREAD_OPERATION_SUPPORT.delete) {
      assertThreadOperationSupported(
        'deleteThread is not implemented by the backend yet',
        'delete',
        remoteId,
        false,
      )
    }
    await deleteThread(remoteId)
  },

  async generateTitle(_remoteId, unstable_messages) {
    const title = generateConversationTitle(unstable_messages)
    return createAssistantStream((controller) => {
      controller.appendText(title)
    })
  },
}

function generateConversationTitle(messages: readonly ThreadMessage[]): string {
  const firstUserMessage = messages.find((message) => message.role === 'user')
  const text = firstUserMessage
    ? firstUserMessage.content
        .filter((part) => part.type === 'text')
        .map((part) => part.text)
        .join(' ')
    : ''

  const normalized = text.replace(/\s+/g, ' ').trim()
  if (!normalized) return 'New Chat'
  if (normalized.length <= 60) return normalized
  return `${normalized.slice(0, 57).trimEnd()}...`
}

function assertThreadOperationSupported(
  message: string,
  operation: string,
  threadId: string,
  supported: boolean,
): void {
  if (supported) return
  throw new Error(`${message} (operation=${operation}, threadId=${threadId})`)
}
