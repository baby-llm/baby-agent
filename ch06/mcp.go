package ch06

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go/v3"
	shared2 "github.com/openai/openai-go/v3/shared"

	"babyagent/ch06/tool"
	"babyagent/shared"
)

type McpClient struct {
	name         string
	client       *mcp.Client
	serverConfig shared.McpServerConfig

	session *mcp.ClientSession
	tools   []tool.Tool
}

func initRunningVars() map[string]string {
	runningVars := map[string]string{
		"${workspaceFolder}": shared.GetWorkspaceDir(),
	}
	return runningVars
}

func NewMcpToolProvider(name string, server shared.McpServerConfig) *McpClient {
	return &McpClient{
		name: name,
		client: mcp.NewClient(&mcp.Implementation{
			Name:    "babyagent-mcp-client",
			Title:   "BabyAgent",
			Version: "v1.0.0",
		}, nil),
		serverConfig: server.ReplacePlaceholders(initRunningVars()),
		tools:        make([]tool.Tool, 0),
	}
}

func (e *McpClient) Name() string {
	return e.name
}

func (e *McpClient) connect(ctx context.Context) error {
	// 服务联通，不需要再初始化
	if e.session != nil {
		if e.session.Ping(ctx, &mcp.PingParams{}) == nil {
			return nil
		}
		// Ping failed: close the stale session before replacing it, otherwise
		// the old stdio process or HTTP connection leaks.
		_ = e.Close()
	}

	var err error
	if e.serverConfig.IsStdio() {
		cmd := exec.Command(e.serverConfig.Command, e.serverConfig.Args...)
		if len(e.serverConfig.Env) > 0 {
			// Keep the parent environment (PATH, HOME, ...) and add overrides.
			// Setting cmd.Env to only the configured map breaks `npx`/`node`.
			cmd.Env = os.Environ()
			for k, v := range e.serverConfig.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
		e.session, err = e.client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	} else {
		httpClient := http.DefaultClient
		if len(e.serverConfig.Headers) > 0 {
			httpClient = &http.Client{
				Transport: &headerRoundTripper{
					base:    http.DefaultTransport,
					headers: e.serverConfig.Headers,
				},
			}
		}
		e.session, err = e.client.Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:   e.serverConfig.Url,
			HTTPClient: httpClient,
		}, nil)
	}
	if err != nil {
		return err
	}

	return nil
}

// Close releases the MCP session and the underlying stdio process / HTTP
// connection. It is safe to call multiple times.
func (e *McpClient) Close() error {
	if e.session == nil {
		return nil
	}
	err := e.session.Close()
	e.session = nil
	e.tools = nil
	return err
}

func (e *McpClient) RefreshTools(ctx context.Context) error {
	if err := e.connect(ctx); err != nil {
		return err
	}

	mcpToolResult, err := e.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return err
	}

	e.tools = make([]tool.Tool, 0)
	for _, mcpTool := range mcpToolResult.Tools {
		agentTool := &McpTool{
			client:   e,
			toolName: mcpTool.Name,
			mcpTool:  mcpTool,
		}

		e.tools = append(e.tools, agentTool)
	}
	return nil
}

func (e *McpClient) GetTools() []tool.Tool {
	return e.tools
}

func (e *McpClient) callTool(ctx context.Context, toolName string, argumentsInJSON string) (string, error) {
	if err := e.connect(ctx); err != nil {
		return "", err
	}
	mcpResult, err := e.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: json.RawMessage(argumentsInJSON),
	})
	if err != nil {
		log.Printf("failed to call tool: %v", err)
		return "", err
	}

	var builder strings.Builder
	for _, content := range mcpResult.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			builder.WriteString(c.Text)
		default:
			builder.WriteString(fmt.Sprintf("[non-text content: %T]", c))
		}
	}
	if mcpResult.IsError {
		return builder.String(), fmt.Errorf("mcp tool %s returned an error: %s", toolName, builder.String())
	}
	return builder.String(), nil
}

// McpTool 实现 tool.Tool 接口
type McpTool struct {
	toolName string // 给 mcp server 看的，和给模型看的不一样
	client   *McpClient
	mcpTool  *mcp.Tool
}

// ToolName 给模型看的，和给 mcp server 看的不一样
func (t *McpTool) ToolName() tool.AgentTool {
	return tool.AgentTool(fmt.Sprintf("babyagent_mcp__%s__%s", t.client.Name(), t.toolName))
}

func (t *McpTool) Info() openai.ChatCompletionToolUnionParam {
	schema, ok := t.mcpTool.InputSchema.(map[string]any)
	if !ok {
		// A server may omit inputSchema or send a non-object schema. Falling
		// back keeps the tool list usable instead of panicking.
		log.Printf("mcp tool %s has non-object input schema %T; using an empty object schema", t.toolName, t.mcpTool.InputSchema)
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return openai.ChatCompletionFunctionTool(shared2.FunctionDefinitionParam{
		Description: openai.String(t.mcpTool.Description),
		Name:        string(t.ToolName()),
		Parameters:  schema,
	})
}

func (t *McpTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	return t.client.callTool(ctx, t.toolName, argumentsInJSON)
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.base.RoundTrip(req)
}
