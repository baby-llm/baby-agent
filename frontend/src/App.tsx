import AssistantThread from './components/assistant-ui/thread'
import AssistantThreadList from './components/assistant-ui/thread-list'
import { BabyAgentRuntimeProvider } from './components/assistant-ui/runtime-provider'

export default function App() {
  return (
    <BabyAgentRuntimeProvider>
      <div style={{ display: 'flex', width: '100%', height: '100vh', overflow: 'hidden' }}>
        <AssistantThreadList />
        <AssistantThread />
      </div>
    </BabyAgentRuntimeProvider>
  )
}
