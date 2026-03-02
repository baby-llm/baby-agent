package context

// StrategyEventType 策略事件类型
type StrategyEventType int

const (
	StrategyEventStart    StrategyEventType = iota // 策略开始执行
	StrategyEventComplete                          // 策略执行完成
)

// StrategyEvent 策略执行事件
type StrategyEvent struct {
	Type  StrategyEventType // 事件类型
	Name  string            // 策略名称
	Error error             // 错误
}
