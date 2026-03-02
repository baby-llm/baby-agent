package context

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"babyagent/shared"
)

type SummaryStrategy struct {
	KeepRecentCount int
	SummaryLength   int
	SummaryBatch    int
	Threshold       float64

	llmClient        openai.Client
	summaryModelConf shared.ModelConfig
}

func (s *SummaryStrategy) Name() string {
	return "summary"
}

func NewSummaryStrategy(summaryModelConf shared.ModelConfig, keepRecentCount, summaryLength, summaryBatch int, threshold float64) *SummaryStrategy {
	llmClient := openai.NewClient(option.WithBaseURL(summaryModelConf.BaseURL), option.WithAPIKey(summaryModelConf.ApiKey))
	return &SummaryStrategy{
		llmClient:        llmClient,
		summaryModelConf: summaryModelConf,
		KeepRecentCount:  keepRecentCount,
		SummaryLength:    summaryLength,
		SummaryBatch:     summaryBatch,
		Threshold:        threshold,
	}
}

func (s *SummaryStrategy) ShouldApply(ctx context.Context, engine *ContextEngine) bool {
	return engine.GetContextUsage() > s.Threshold
}

func (s *SummaryStrategy) Apply(ctx context.Context, engine *ContextEngine) error {
	if len(engine.messages) <= s.KeepRecentCount {
		return nil
	}

	toSummaryIndex := len(engine.messages) - s.KeepRecentCount
	triggerThreshold := s.summaryModelConf.ContextWindow / 2

	accumulatedSummary := ""

	// 计算被替换消息的总 token 数
	removedTokens := 0
	for i := 0; i < toSummaryIndex; i++ {
		removedTokens += CountTokens(engine.messages[i])
	}

	batchStart := 0

	for batchStart < toSummaryIndex {
		batchMessages := make([]openai.ChatCompletionMessageParamUnion, 0)
		batchTokens := 0

		for i := batchStart; i < toSummaryIndex; i++ {
			// 计算当前消息的 token 数
			msgTokens := CountTokens(engine.messages[i])

			// 如果加上这条消息后超过阈值，且已经有消息了，则停止添加
			if batchTokens+msgTokens > triggerThreshold && len(batchMessages) > 0 {
				break
			}

			batchMessages = append(batchMessages, engine.messages[i])
			batchTokens += msgTokens

			// 达到 batch 数量，停止添加
			if len(batchMessages) >= s.SummaryBatch {
				break
			}
		}

		if len(batchMessages) == 0 {
			break
		}

		batchSummary, err := s.generateSummary(ctx, batchMessages, accumulatedSummary)
		if err != nil {
			return err
		}

		accumulatedSummary = batchSummary
		batchStart += len(batchMessages)
	}

	if len(accumulatedSummary) == 0 {
		log.Printf("no summary generated")
		return nil
	}

	// 构建新的消息列表
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(engine.messages))
	summaryMessage := openai.UserMessage(accumulatedSummary)
	messages = append(messages, summaryMessage)
	messages = append(messages, engine.messages[toSummaryIndex:]...)

	// 计算新摘要消息的 token 数
	newTokens := CountTokens(summaryMessage)

	// 更新消息列表和 token 计数
	engine.messages = messages
	engine.contextTokens = engine.contextTokens - removedTokens + newTokens

	return nil
}

func (s *SummaryStrategy) generateSummary(ctx context.Context, batchMessages []openai.ChatCompletionMessageParamUnion, accumulatedSummary string) (string, error) {
	var b strings.Builder

	b.WriteString(accumulatedSummary)
	for i := range batchMessages {
		contentAny := batchMessages[i].GetContent().AsAny()
		contentStr, ok := contentAny.(*string)
		if !ok {
			continue
		}
		b.WriteString(*batchMessages[i].GetRole())
		b.WriteString(": ")
		b.WriteString(*contentStr)
		b.WriteString("\n")
	}

	prompt := strings.ReplaceAll(summaryPromptTemplate, "{text}", b.String())
	prompt = strings.ReplaceAll(prompt, "{summary_length}", strconv.Itoa(s.SummaryLength))

	resp, err := s.llmClient.Chat.Completions.New(ctx,
		openai.ChatCompletionNewParams{
			Model: s.summaryModelConf.Model,
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(prompt),
			},
		},
	)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no choices returned")
	}
	return resp.Choices[0].Message.Content, nil

}

const (
	summaryPromptTemplate = `Summarize the following conversation history between a user and an AI assistant.

<conversation>
{text}
</conversation>

Requirements:
- Preserve key information: user requests, tool calls, and important results
- Keep the summary under {summary_length} characters
- Output ONLY the summary, no explanations
- Use concise language, omit redundant details

Example:

Input:
user: What files are in the current directory?
assistant: I'll use the bash tool to list files.
tool: file1.txt file2.go directory/
assistant: The directory contains file1.txt, file2.go, and a directory/.

Output:
User asked to list directory contents. Assistant ran bash command showing file1.txt, file2.go, and a directory.
`
)
