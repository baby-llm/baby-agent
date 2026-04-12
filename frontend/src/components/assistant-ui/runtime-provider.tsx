import { AssistantRuntimeProvider, useLocalRuntime, useRemoteThreadListRuntime } from '@assistant-ui/react'
import type { PropsWithChildren } from 'react'

import { babyAgentChatModelAdapter } from './model-adapter'
import { babyAgentThreadListAdapter } from './thread-list-adapter'

function RuntimeRoot({ children }: PropsWithChildren) {
  const runtime = useRemoteThreadListRuntime({
    runtimeHook: function BabyAgentLocalRuntime() {
      return useLocalRuntime(babyAgentChatModelAdapter)
    },
    adapter: babyAgentThreadListAdapter,
  })

  return <AssistantRuntimeProvider runtime={runtime}>{children}</AssistantRuntimeProvider>
}

export function BabyAgentRuntimeProvider({ children }: PropsWithChildren) {
  return <RuntimeRoot>{children}</RuntimeRoot>
}
