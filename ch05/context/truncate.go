package context

import "context"

type TruncateStrategy struct {
	KeepRecentCount int     // 保留最近 N 条消息
	MinMessageCount int     // 最少保留的消息数量
	Threshold       float64 // 触发阈值
}

func NewTruncateStrategy(keepRecentCount, minMessageCount int, threshold float64) *TruncateStrategy {
	return &TruncateStrategy{
		KeepRecentCount: keepRecentCount,
		MinMessageCount: minMessageCount,
		Threshold:       threshold,
	}
}

func (s *TruncateStrategy) Name() string {
	return "truncation"
}

func (s *TruncateStrategy) Apply(ctx context.Context, engine *ContextEngine) error {
	if len(engine.messages) <= s.MinMessageCount {
		return nil
	}

	// 准备截断的前 toRemove 条消息
	toRemove := len(engine.messages) - s.KeepRecentCount
	if toRemove <= 0 {
		return nil
	}

	// 在 0 ~ toRemove - 1 中找到最后一次 User 消息，保留这个 User 之后的消息，截断之前所有的历史
	removeIdx := toRemove - 1
	for i := toRemove - 1; i >= 0; i-- {
		role := engine.messages[i].GetRole()
		if role != nil && *role == "user" {
			removeIdx = i
			break
		}
	}

	// 如果没有找到 user 消息，或者 removeIdx 为 0，则不删除任何消息
	// 这样可以确保不会删除所有消息
	if removeIdx <= 0 {
		return nil
	}

	removedTokens := 0
	for i := 0; i < removeIdx; i++ {
		removedTokens += CountTokens(engine.messages[i])
	}
	engine.messages = engine.messages[removeIdx:]
	engine.contextTokens -= removedTokens
	return nil
}

func (s *TruncateStrategy) ShouldApply(ctx context.Context, engine *ContextEngine) bool {
	return engine.GetContextUsage() > s.Threshold
}
