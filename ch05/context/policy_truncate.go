package context

import "context"

type TruncatePolicy struct {
	// KeepRecentMessages 表示最少保留的最近消息数量。
	KeepRecentMessages int
	// UsageThreshold 表示上下文使用率超过该值时触发截断。
	UsageThreshold float64
}

func NewTruncatePolicy(keepRecentMessages int, usageThreshold float64) *TruncatePolicy {
	return &TruncatePolicy{
		KeepRecentMessages: keepRecentMessages,
		UsageThreshold:     usageThreshold,
	}
}

func (p *TruncatePolicy) Name() string {
	return "truncate"
}

func (p *TruncatePolicy) Apply(ctx context.Context, engine *Engine) (PolicyResult, error) {
	if len(engine.messages) <= p.KeepRecentMessages {
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}

	// Preserve a leading summary inserted by SummaryPolicy; it represents
	// history that has already been compressed and is still useful.
	prefix := 0
	if len(engine.messages) > 0 && isSummaryMessage(engine.messages[0].Message) {
		prefix = 1
	}

	// 准备截断的前 toRemove 条消息
	toRemove := len(engine.messages) - p.KeepRecentMessages

	// 在 prefix ~ toRemove - 1 中找到最后一次 User 消息，保留这个 User 之后的消息，截断之前所有的历史
	removeIdx := toRemove - 1
	for i := toRemove - 1; i >= prefix; i-- {
		if engine.messages[i].Message.OfUser != nil {
			removeIdx = i
			break
		}
	}

	// 如果没有找到 user 消息，或者 removeIdx 落在保留前缀内，则不删除任何消息
	// 这样可以确保不会删除所有消息或摘要
	if removeIdx <= prefix {
		return PolicyResult{
			Messages:      engine.messages,
			ContextTokens: engine.contextTokens,
		}, nil
	}

	removedTokens := 0
	for i := prefix; i < removeIdx; i++ {
		removedTokens += engine.messages[i].Tokens
	}

	kept := engine.messages[removeIdx:]
	if prefix == 1 {
		kept = append([]messageWrap{engine.messages[0]}, kept...)
	}

	return PolicyResult{
		Messages:      kept,
		ContextTokens: engine.contextTokens - removedTokens,
	}, nil
}

func (p *TruncatePolicy) ShouldApply(ctx context.Context, engine *Engine) bool {
	return engine.GetContextUsage() > p.UsageThreshold
}
