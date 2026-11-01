package shared

import (
	"encoding/json"
	"os"
)

type AppConfig struct {
	LLMProviders struct {
		FrontModel ModelConfig `json:"front_model"`
		BackModel  ModelConfig `json:"back_model"`
	} `json:"llm_providers"`
}

type ModelConfig struct {
	BaseURL string `json:"base_url"`
	ApiKey  string `json:"api_key"`
	Model   string `json:"model"`

	ContextWindow int `json:"context_window"`
}

func NewModelConfig() ModelConfig {
	return ModelConfig{
		BaseURL:       getEnvDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		ApiKey:        getEnvDefault("OPENAI_API_KEY", ""),
		Model:         getEnvDefault("OPENAI_MODEL", "gpt-5.2"),
		ContextWindow: 200000,
	}
}

func getEnvDefault(key, defaultValue string) string {
	// Empty values (e.g. `KEY=` in .env.example) intentionally fall back to the
	// default, so copying the example file and only filling the API key works.
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return defaultValue
}

func LoadAppConfig(path string) (AppConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, err
	}
	var config AppConfig
	err = json.Unmarshal(content, &config)
	if err != nil {
		return AppConfig{}, err
	}
	return config, nil
}
