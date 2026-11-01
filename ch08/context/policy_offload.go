package context

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"

	"babyagent/ch08/storage"
	"babyagent/shared"
)

type OffloadPolicy struct {
	// Storage 用于保存被卸载的长文本内容。
	Storage storage.Storage
	// UsageThreshold 表示上下文使用率超过该值时触发卸载。
	UsageThreshold float64
	// KeepRecentMessages 表示跳过最后 N 条消息，避免影响最新对话。
	KeepRecentMessages int
	// PreviewCharLimit 表示卸载后在上下文里保留的字符数。
	PreviewCharLimit int
}

func NewOffloadPolicy(storage storage.Storage, usageThreshold float64, keepRecentMessages, previewCharLimit int) *OffloadPolicy {
	return &OffloadPolicy{
		Storage:            storage,
		UsageThreshold:     usageThreshold,
		KeepRecentMessages: keepRecentMessages,
		PreviewCharLimit:   previewCharLimit,
	}
}

func (p *OffloadPolicy) Name() string {
	return "offload"
}

func (p *OffloadPolicy) makeStorageKey(offloadIndex int) string {
	return fmt.Sprintf("/offload/%d_%d", time.Now().UnixNano(), offloadIndex)
}

func (p *OffloadPolicy) Apply(ctx context.Context, engine *Engine) (PolicyResult, error) {
	if len(engine.messages) <= p.KeepRecentMessages {
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}

	// 复制消息列表，避免修改原始数据
	messages := make([]messageWrap, len(engine.messages))
	copy(messages, engine.messages)
	contextTokens := engine.contextTokens

	offloadCount := len(messages) - p.KeepRecentMessages

	for i := 0; i < offloadCount; i++ {
		// 只卸载 tool 类型
		if shared.GetRoleName(messages[i].Message) != "tool" {
			continue
		}

		contentAny := messages[i].Message.GetContent().AsAny()
		contentStr, ok := contentAny.(*string)
		if !ok {
			continue
		}
		// 不需要卸载（按 rune 比较，避免把中文/emoji 截断成半个字符）
		runes := []rune(*contentStr)
		if len(runes) <= p.PreviewCharLimit {
			continue
		}

		// 计算原始消息的 token 数
		oldTokens := messages[i].Tokens

		key := p.makeStorageKey(i)
		if err := p.Storage.Store(ctx, key, *contentStr); err != nil {
			log.Printf("failed to store offload message: %v", err)
			continue
		}

		// 构造卸载后的消息体正文
		abstract := string(runes[:p.PreviewCharLimit])
		var b strings.Builder
		b.WriteString(abstract)
		b.WriteString("...")
		b.WriteString(fmt.Sprintf("（更多内容已卸载，如需查看全文请使用 load_storage(key=\"%s\") 工具）\n", key))
		newContent := b.String()

		// 修改原始消息链中的消息
		newMessage := openai.ToolMessage(newContent, engine.messages[i].Message.OfTool.ToolCallID)

		// 计算新消息的 token 数并更新计数
		newTokens := CountTokens(newMessage)
		messages[i] = messageWrap{Message: newMessage, Tokens: newTokens}
		contextTokens -= oldTokens - newTokens
	}

	return PolicyResult{
		Messages:      messages,
		ContextTokens: contextTokens,
	}, nil
}

func (p *OffloadPolicy) ShouldApply(ctx context.Context, engine *Engine) bool {
	return engine.GetContextUsage() > p.UsageThreshold
}
