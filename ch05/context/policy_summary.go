package context

import (
	"context"
	"log"
	"strings"

	"github.com/openai/openai-go/v3"

	"babyagent/shared"
)

// summaryMessagePrefix marks the synthetic summary message so the truncate
// policy can preserve it instead of removing it with older history.
const summaryMessagePrefix = "[Conversation summary so far]\n"

type SummaryPolicy struct {
	// KeepRecentMessages 表示跳过最后 N 条消息，避免摘要最新对话。
	KeepRecentMessages int
	// SummaryBatchSize 表示单次送入摘要器的最大消息数。
	SummaryBatchSize int
	// UsageThreshold 表示上下文使用率超过该值时触发摘要。
	UsageThreshold float64
	// Summarizer 负责执行摘要生成。
	Summarizer Summarizer
}

func (p *SummaryPolicy) Name() string {
	return "summarize"
}

func NewSummaryPolicy(summarizer Summarizer, keepRecentMessages, summaryBatchSize int, usageThreshold float64) *SummaryPolicy {
	return &SummaryPolicy{
		KeepRecentMessages: keepRecentMessages,
		Summarizer:         summarizer,
		SummaryBatchSize:   summaryBatchSize,
		UsageThreshold:     usageThreshold,
	}
}

func (p *SummaryPolicy) ShouldApply(ctx context.Context, engine *Engine) bool {
	return engine.GetContextUsage() > p.UsageThreshold
}

func (p *SummaryPolicy) Apply(ctx context.Context, engine *Engine) (PolicyResult, error) {
	if len(engine.messages) <= p.KeepRecentMessages {
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}

	summarizeUntilIndex := len(engine.messages) - p.KeepRecentMessages
	if summarizeUntilIndex >= len(engine.messages) {
		summarizeUntilIndex = len(engine.messages) - 1
	}
	// Never let the kept tail start with a tool result: the matching
	// assistant(tool_calls) message would be summarized away, producing an
	// invalid request. Move the boundary back to the assistant tool-call message.
	for summarizeUntilIndex > 0 && engine.messages[summarizeUntilIndex].Message.OfTool != nil {
		summarizeUntilIndex--
	}
	if summarizeUntilIndex <= 0 {
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}

	inputTokenLimit := p.Summarizer.GetSummaryInputTokenLimit()

	accumulatedSummary := ""

	// 计算被替换消息的总 token 数
	removedTokens := 0
	for i := 0; i < summarizeUntilIndex; i++ {
		removedTokens += engine.messages[i].Tokens
	}

	batchStart := 0

	for batchStart < summarizeUntilIndex {
		batchMessages := make([]shared.OpenAIMessage, 0)
		batchTokens := 0

		for i := batchStart; i < summarizeUntilIndex; i++ {
			// 计算当前消息的 token 数
			msgTokens := engine.messages[i].Tokens

			// 如果加上这条消息后超过阈值，且已经有消息了，则停止添加
			if batchTokens+msgTokens > inputTokenLimit && len(batchMessages) > 0 {
				break
			}

			batchMessages = append(batchMessages, engine.messages[i].Message)
			batchTokens += msgTokens

			// 达到 batch 数量，停止添加
			if len(batchMessages) >= p.SummaryBatchSize {
				break
			}
		}

		if len(batchMessages) == 0 {
			break
		}

		batchSummary, err := p.Summarizer.Summarize(ctx, accumulatedSummary, batchMessages)
		if err != nil {
			return PolicyResult{}, err
		}

		accumulatedSummary = batchSummary
		batchStart += len(batchMessages)
	}

	if len(accumulatedSummary) == 0 {
		log.Printf("no summary generated")
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}

	// 构建新的消息列表
	messages := make([]messageWrap, 0, len(engine.messages))

	summaryMessage := openai.UserMessage(summaryMessagePrefix + accumulatedSummary)
	newTokens := CountTokens(summaryMessage)

	messages = append(messages, messageWrap{Message: summaryMessage, Tokens: newTokens})
	messages = append(messages, engine.messages[summarizeUntilIndex:]...)

	// 返回新的消息列表和 token 计数
	return PolicyResult{
		Messages:      messages,
		ContextTokens: engine.contextTokens - removedTokens + newTokens,
	}, nil
}

// isSummaryMessage reports whether a message is the synthetic summary inserted
// by SummaryPolicy.
func isSummaryMessage(message shared.OpenAIMessage) bool {
	content, ok := message.GetContent().AsAny().(*string)
	return ok && strings.HasPrefix(*content, summaryMessagePrefix)
}
