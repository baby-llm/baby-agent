package shared

import (
	"sort"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// NewLLMClient creates the one client used by every chapter that talks to an
// OpenAI-compatible endpoint. A request timeout keeps a hung provider from
// blocking an agent turn forever; retries are left to the SDK's default.
func NewLLMClient(modelConf ModelConfig) openai.Client {
	opts := []option.RequestOption{
		option.WithBaseURL(modelConf.BaseURL),
		option.WithAPIKey(modelConf.ApiKey),
		option.WithRequestTimeout(2 * time.Minute),
		option.WithHeader("X-Title", "BabyAgent"),
		option.WithHeader("HTTP-Referer", "https://github.com/baby-llm/baby-agent"),
	}

	keys := make([]string, 0, len(modelConf.ExtraBody))
	for k := range modelConf.ExtraBody {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		opts = append(opts, option.WithJSONSet(k, modelConf.ExtraBody[k]))
	}

	return openai.NewClient(opts...)
}
