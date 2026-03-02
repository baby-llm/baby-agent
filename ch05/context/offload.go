package context

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

type OffloadStrategy struct {
	Threshold       float64
	KeepRecentCount int
	AbstractLength  int // 卸载之后保留的消息长度
	StoragePrefix   string
}

func NewOffloadStrategy(threshold float64, keepRecentCount, abstractLength int, storagePrefix string) *OffloadStrategy {
	return &OffloadStrategy{
		Threshold:       threshold,
		KeepRecentCount: keepRecentCount,
		AbstractLength:  abstractLength,
		StoragePrefix:   storagePrefix,
	}
}

func (s *OffloadStrategy) Name() string {
	return "offloading"
}

func (s *OffloadStrategy) makeStorageKey(i int) string {
	return fmt.Sprintf("/offload/%s_%s_%d", s.StoragePrefix, time.Now().Format("20060102_150405"), i)
}

func (s *OffloadStrategy) Apply(ctx context.Context, engine *ContextEngine) error {
	if len(engine.messages) <= s.KeepRecentCount {
		return nil
	}

	offloadCount := len(engine.messages) - s.KeepRecentCount

	for i := 0; i < offloadCount; i++ {
		contentAny := engine.messages[i].GetContent().AsAny()
		contentStr, ok := contentAny.(*string)
		if !ok {
			continue
		}
		// 不需要卸载
		if len(*contentStr) <= s.AbstractLength {
			continue
		}

		// 计算原始消息的 token 数
		oldTokens := CountTokens(engine.messages[i])

		key := s.makeStorageKey(i)
		if err := engine.storage.Store(ctx, key, *contentStr); err != nil {
			log.Printf("failed to store offload message: %v", err)
			continue
		}

		// 构造卸载后的消息体正文
		abstract := (*contentStr)[0:s.AbstractLength]
		var b strings.Builder
		b.WriteString(abstract)
		b.WriteString("...")
		b.WriteString(fmt.Sprintf("（更多内容已卸载，如需查看全文请使用 load_storage(key=\"%s\") 工具）\n", key))
		newContent := b.String()

		// 修改原始消息链中的消息
		var newMessage openai.ChatCompletionMessageParamUnion
		switch *engine.messages[i].GetRole() {
		case "user":
			newMessage = openai.UserMessage(newContent)
		case "assistant":
			newMessage = openai.AssistantMessage(newContent)
		case "tool":
			newMessage = openai.ToolMessage(newContent, *engine.messages[i].GetToolCallID())
		default:
			continue
		}

		// 计算新消息的 token 数并更新计数
		newTokens := CountTokens(newMessage)
		engine.messages[i] = newMessage
		engine.contextTokens -= oldTokens - newTokens
	}
	return nil
}

func (s *OffloadStrategy) ShouldApply(ctx context.Context, engine *ContextEngine) bool {
	return engine.GetContextUsage() > s.Threshold
}
