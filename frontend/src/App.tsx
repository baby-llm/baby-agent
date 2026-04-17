import { useState, useEffect } from 'react'
import Sidebar from './components/Sidebar'
import ChatPanel from './components/ChatPanel'
import AssistantThread from './components/assistant-ui/thread'
import AssistantThreadList from './components/assistant-ui/thread-list'
import { BabyAgentRuntimeProvider } from './components/assistant-ui/runtime-provider'
import {
  listConversations,
  createConversation,
  type ConversationVO,
} from './api'

// ch12 needs a simple chapter-oriented layout with a dedicated plan panel.
// Keep the legacy flow active until planning state is wired into assistant-ui.
const USE_ASSISTANT_UI = false

export default function App() {
  return USE_ASSISTANT_UI ? <AssistantUIApp /> : <LegacyApp />
}

function LegacyApp() {
  const [conversations, setConversations] = useState<ConversationVO[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  // null means "pending new chat" (no conversation created yet)
  const [pendingNew, setPendingNew] = useState(false)

  useEffect(() => {
    listConversations()
      .then((data) => {
        setConversations(data)
        if (data.length > 0) setActiveId(data[0].conversation_id)
      })
      .catch(console.error)
  }, [])

  // Called by Sidebar "New Chat" button — just enter pending state
  const handleNew = () => {
    setActiveId(null)
    setPendingNew(true)
  }

  // Called by ChatPanel when the user sends their first message in a pending chat
  const handleConversationCreated = (conv: ConversationVO) => {
    setConversations((prev) => [conv, ...prev])
    setActiveId(conv.conversation_id)
    setPendingNew(false)
  }

  return (
    <div style={{ display: 'flex', width: '100%', height: '100vh', overflow: 'hidden' }}>
      <Sidebar
        conversations={conversations}
        activeId={activeId}
        onSelect={(id) => { setActiveId(id); setPendingNew(false) }}
        onNew={handleNew}
      />
      <div style={{ flex: 1, overflow: 'hidden' }}>
        {activeId ? (
          <ChatPanel key={activeId} conversationId={activeId} />
        ) : (
          <ChatPanel
            key="pending"
            conversationId={null}
            onConversationCreated={handleConversationCreated}
          />
        )}
      </div>
    </div>
  )
}

function AssistantUIApp() {
  return (
    <BabyAgentRuntimeProvider>
      <div style={{ display: 'flex', width: '100%', height: '100vh', overflow: 'hidden' }}>
        <AssistantThreadList />
        <AssistantThread />
      </div>
    </BabyAgentRuntimeProvider>
  )
}
