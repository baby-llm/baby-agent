package server

import (
	"babyagent/shared"
	"encoding/json"
)

func buildHistory(all []ChatMessage, parent string) []shared.OpenAIMessage {
	if parent == "" {
		return nil
	}
	index := map[string]*ChatMessage{}
	for i := range all {
		index[all[i].MessageID] = &all[i]
	}
	var path []*ChatMessage
	for parent != "" {
		m, ok := index[parent]
		if !ok {
			break
		}
		path = append(path, m)
		parent = m.ParentMessageID
	}
	for l, r := 0, len(path)-1; l < r; l, r = l+1, r-1 {
		path[l], path[r] = path[r], path[l]
	}
	var history []shared.OpenAIMessage
	for _, m := range path {
		var rounds []shared.OpenAIMessage
		if json.Unmarshal([]byte(m.Rounds), &rounds) == nil {
			history = append(history, rounds...)
		}
	}
	return history
}
