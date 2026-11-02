package shared

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
)

type McpServerConfig struct {
	// for stdio
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	// for http
	Type    string            `json:"type"`
	Url     string            `json:"url"`
	Headers map[string]string `json:"headers"`
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

// replacePlaceholders replaces longer keys first. Otherwise a map containing
// both "${workspace}" and "${workspaceFolder}" can expand the short key inside
// the long one and corrupt the result.
func replacePlaceholders(value string, replaceMap map[string]string) string {
	keys := make([]string, 0, len(replaceMap))
	for k := range replaceMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	for _, k := range keys {
		value = strings.ReplaceAll(value, k, replaceMap[k])
	}
	return value
}

func LoadMcpServerConfig(path string) (map[string]McpServerConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	serverMap := make(map[string]McpServerConfig)

	if err := json.Unmarshal(content, &serverMap); err != nil {
		return nil, err
	}
	for name, server := range serverMap {
		if !server.IsStdio() && !server.IsHttp() {
			return nil, errors.New("MCP server " + name + " must set either command (stdio) or url (http)")
		}
	}
	return serverMap, nil
}
