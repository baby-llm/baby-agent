package server

import (
	"encoding/json"

	"babyagent/shared"
)

// buildHistory 根据 parent_message_id 沿树向上追溯路径，
// 将路径上每条消息的 rounds 拼接成 LLM history。
// allMsgs 为该会话下的全部消息，parentMessageID 为本次请求的父消息 ID。
func buildHistory(allMsgs []ChatMessage, parentMessageID string) []shared.OpenAIMessage {
	if parentMessageID == "" {
		return nil
	}

	// 构建 id -> message 索引
	index := make(map[string]*ChatMessage, len(allMsgs))
	for i := range allMsgs {
		index[allMsgs[i].MessageID] = &allMsgs[i]
	}

	// 从 parentMessageID 向根节点追溯，收集路径（顺序：根 -> parent）。
	// visited 防止脏数据里的环导致死循环。
	path := make([]*ChatMessage, 0)
	visited := make(map[string]bool, len(allMsgs))
	cur := parentMessageID
	for cur != "" && !visited[cur] {
		msg, ok := index[cur]
		if !ok {
			break
		}
		visited[cur] = true
		path = append(path, msg)
		cur = msg.ParentMessageID
	}

	// 反转：变为根 -> parent 顺序
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	// 拼接每条消息的 rounds
	history := make([]shared.OpenAIMessage, 0)
	for _, msg := range path {
		if msg.Rounds == "" {
			continue
		}
		var rounds []shared.OpenAIMessage
		if err := json.Unmarshal([]byte(msg.Rounds), &rounds); err != nil {
			continue
		}
		history = append(history, rounds...)
	}
	return history
}
