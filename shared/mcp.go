package shared

import (
	"encoding/json"
	"os"
	"strings"
)

type McpServerConfig struct {
	// for stdio
	Command string            `json:"command" yaml:"command"`
	Args    []string          `json:"args" yaml:"args"`
	Env     map[string]string `json:"env" yaml:"env"`
	// for http
	Type    string            `json:"type" yaml:"type"`
	Url     string            `json:"url" yaml:"url"`
	Headers map[string]string `json:"headers" yaml:"headers"`
}

func (s *McpServerConfig) IsStdio() bool {
	return s.Command != ""
}

func (s *McpServerConfig) IsHttp() bool {
	return s.Url != ""
}

func (s *McpServerConfig) ReplacePlaceholders(replaceMap map[string]string) McpServerConfig {
	newConfig := McpServerConfig{
		Command: replacePlaceholders(s.Command, replaceMap),
		Args:    make([]string, 0, len(s.Args)),
		Type:    s.Type,
		Url:     replacePlaceholders(s.Url, replaceMap),
	}

	for _, arg := range s.Args {
		newConfig.Args = append(newConfig.Args, replacePlaceholders(arg, replaceMap))
	}

	// Copy maps instead of sharing them with the original config. Replacing
	// placeholders in Env/Headers matters for HTTP servers that authenticate
	// with a token or pass a workspace path to the child process.
	if s.Env != nil {
		newConfig.Env = make(map[string]string, len(s.Env))
		for k, v := range s.Env {
			newConfig.Env[k] = replacePlaceholders(v, replaceMap)
		}
	}
	if s.Headers != nil {
		newConfig.Headers = make(map[string]string, len(s.Headers))
		for k, v := range s.Headers {
			newConfig.Headers[k] = replacePlaceholders(v, replaceMap)
		}
	}

	return newConfig
}

func replacePlaceholders(value string, replaceMap map[string]string) string {
	for k, v := range replaceMap {
		value = strings.ReplaceAll(value, k, v)
	}
	return value
}

func LoadMcpServerConfig(path string) (map[string]McpServerConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	serverMap := make(map[string]McpServerConfig)

	err = json.Unmarshal(content, &serverMap)
	if err != nil {
		return nil, err
	}
	return serverMap, nil
}
