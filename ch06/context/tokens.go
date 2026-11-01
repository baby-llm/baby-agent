package context

import (
	"encoding/json"
	"sync"

	"github.com/tiktoken-go/tokenizer"

	"babyagent/shared"
)

// perMessageTokenOverhead is a rough allowance for role markers, message
// delimiters and tool-call scaffolding. It keeps the estimate from looking
// unrealistically small for short messages.
const perMessageTokenOverhead = 4

var (
	tokenEncOnce sync.Once
	tokenEnc     tokenizer.Codec
)

func getTokenEncoder() tokenizer.Codec {
	tokenEncOnce.Do(func() {
		// Loading the encoding is a local operation; if it ever fails we fall
		// back to a rough character estimate instead of exiting the process.
		tokenEnc, _ = tokenizer.Get(tokenizer.Cl100kBase)
	})
	return tokenEnc
}

func CountTokens(message shared.OpenAIMessage) int {
	count := perMessageTokenOverhead

	if content, ok := message.GetContent().AsAny().(*string); ok {
		count += countText(*content)
	}

	// Assistant tool calls live outside the content field. Without counting
	// their JSON, a turn with large tool arguments looks nearly free.
	if message.OfAssistant != nil && len(message.OfAssistant.ToolCalls) > 0 {
		if raw, err := json.Marshal(message.OfAssistant.ToolCalls); err == nil {
			count += countText(string(raw))
		}
	}

	return count
}

func countText(text string) int {
	enc := getTokenEncoder()
	if enc == nil {
		// ~4 characters per token is the usual rule of thumb for English.
		return len([]rune(text)) / 4
	}
	count, _ := enc.Count(text)
	return count
}
