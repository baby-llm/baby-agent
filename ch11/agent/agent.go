package agent

import (
	"babyagent/ch11/agent/tool"
	"babyagent/ch11/observe"
	"babyagent/shared"
	"context"
	"encoding/json"
	"fmt"
	"github.com/openai/openai-go/v3"
	"go.opentelemetry.io/otel/attribute"
	"time"
)

const SystemPrompt = `# BabyAgent

You are BabyAgent, a helpful coding assistant.

## Guidelines
- State intent before tool calls, but NEVER predict or claim results before receiving them.
- Before modifying a file, read it first. Do not assume files or directories exist.
- If a tool call fails, analyze the error before retrying with a different approach.
- Ask for clarification when the request is ambiguous.

Reply directly with text for conversations.`

type Agent struct {
	model        string
	client       openai.Client
	nativeTools  map[tool.AgentTool]tool.Tool
	systemPrompt string
	metrics      *observe.Metrics
}

func NewAgent(conf shared.ModelConfig, prompt string, tools []tool.Tool, metrics *observe.Metrics) *Agent {
	a := &Agent{model: conf.Model, client: shared.NewLLMClient(conf), nativeTools: map[tool.AgentTool]tool.Tool{}, systemPrompt: prompt, metrics: metrics}
	for _, t := range tools {
		a.nativeTools[t.ToolName()] = t
	}
	return a
}
func (a *Agent) Model() string { return a.model }
func (a *Agent) buildTools() []openai.ChatCompletionToolUnionParam {
	r := make([]openai.ChatCompletionToolUnionParam, 0, len(a.nativeTools))
	for _, t := range a.nativeTools {
		r = append(r, t.Info())
	}
	return r
}

type RunResult struct {
	Response string
	Rounds   []shared.OpenAIMessage
	Usage    openai.CompletionUsage
}

func (a *Agent) RunStreaming(ctx context.Context, history []openai.ChatCompletionMessageParamUnion, query string, events chan<- StreamEvent, trace *observe.AgentTrace) (RunResult, error) {
	messages := append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(a.systemPrompt)}, history...)
	messages = append(messages, openai.UserMessage(query))
	rounds := []shared.OpenAIMessage{openai.UserMessage(query)}
	var usage openai.CompletionUsage
	var response string
	for loop := 1; ; loop++ {
		llmStarted := time.Now()
		llmCtx, span := observe.StartLLMSpan(ctx, a.model, loop)
		stream := a.client.Chat.Completions.NewStreaming(llmCtx, openai.ChatCompletionNewParams{Model: a.model, Messages: messages, Tools: a.buildTools(), StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}})
		acc := openai.ChatCompletionAccumulator{}
		var firstToken time.Time
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			if len(chunk.Choices) == 0 {
				continue
			}
			raw := chunk.Choices[0].Delta
			var delta deltaWithReasoning
			_ = json.Unmarshal([]byte(raw.RawJSON()), &delta)
			if (delta.Content != "" || delta.ReasoningContent != "") && firstToken.IsZero() {
				firstToken = time.Now()
			}
			if delta.ReasoningContent != "" {
				events <- StreamEvent{Event: EventReasoning, ReasoningContent: delta.ReasoningContent}
			}
			if delta.Content != "" {
				events <- StreamEvent{Event: EventContent, Content: delta.Content}
			}
		}
		llmDuration := time.Since(llmStarted)
		firstTokenDuration := time.Duration(0)
		if !firstToken.IsZero() {
			firstTokenDuration = firstToken.Sub(llmStarted)
		}
		if err := stream.Err(); err != nil {
			span.RecordError(err)
			span.End()
			events <- StreamEvent{Event: EventError, Content: err.Error()}
			return RunResult{}, err
		}
		if len(acc.Choices) == 0 {
			span.End()
			break
		}
		usage = acc.Usage
		message := acc.Choices[0].Message
		span.SetAttributes(attribute.Int("gen_ai.usage.input_tokens", int(usage.PromptTokens)), attribute.Int("gen_ai.usage.output_tokens", int(usage.CompletionTokens)))
		span.End()
		if a.metrics != nil {
			a.metrics.ObserveLLM(a.model, firstTokenDuration, llmDuration-firstTokenDuration, usage.PromptTokens, usage.CompletionTokens)
		}
		trace.LLM(llmDuration, usage.PromptTokens, usage.CompletionTokens, firstTokenDuration, len(message.ToolCalls))
		assistant := message.ToParam()
		messages = append(messages, assistant)
		rounds = append(rounds, assistant)
		if len(message.ToolCalls) == 0 {
			response = message.Content
			break
		}
		for _, call := range message.ToolCalls {
			events <- StreamEvent{Event: EventToolCall, ToolCall: call.Function.Name, ToolArguments: call.Function.Arguments}
			toolStarted := time.Now()
			toolCtx, toolSpan := observe.StartToolSpan(ctx, call.Function.Name)
			t, ok := a.nativeTools[call.Function.Name]
			var result string
			var err error
			if !ok {
				err = fmt.Errorf("tool not found: %s", call.Function.Name)
			} else {
				result, err = t.Execute(toolCtx, call.Function.Arguments)
			}
			duration := time.Since(toolStarted)
			if err != nil {
				result = err.Error()
				toolSpan.RecordError(err)
				events <- StreamEvent{Event: EventError, Content: result}
			}
			toolSpan.End()
			status := "ok"
			if err != nil {
				status = "error"
			}
			if a.metrics != nil {
				a.metrics.ToolCalls.WithLabelValues(call.Function.Name, status).Inc()
				a.metrics.ToolDuration.WithLabelValues(call.Function.Name, status).Observe(duration.Seconds())
			}
			trace.Tool(call.Function.Name, duration, err)
			events <- StreamEvent{Event: EventToolResult, ToolCall: call.Function.Name, ToolResult: result}
			m := openai.ToolMessage(result, call.ID)
			messages = append(messages, m)
			rounds = append(rounds, m)
		}
		select {
		case <-ctx.Done():
			return RunResult{Response: response, Rounds: rounds, Usage: usage}, ctx.Err()
		default:
		}
	}
	return RunResult{Response: response, Rounds: rounds, Usage: usage}, nil
}

type deltaWithReasoning struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}
