package context

import (
	"encoding/json"
	"log"

	"github.com/tiktoken-go/tokenizer"

	"babyagent/shared"
)

// perMessageTokenOverhead is a rough allowance for role markers, message
// delimiters and tool-call scaffolding. It keeps the estimate from looking
// unrealistically small for short messages.
const perMessageTokenOverhead = 4

var tokenEnc tokenizer.Codec

func init() {
	var err error
	tokenEnc, err = tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		log.Fatal(err)
	}
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
	count, _ := tokenEnc.Count(text)
	return count
}
