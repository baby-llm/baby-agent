package ch05

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openai/openai-go/v3"
	shared2 "github.com/openai/openai-go/v3/shared"

	"babyagent/ch05/tool"
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
	if e.session != nil && e.session.Ping(ctx, &mcp.PingParams{}) == nil {
		return nil
	}
	var err error
	if e.serverConfig.IsStdio() {
		cmd := exec.Command(e.serverConfig.Command, e.serverConfig.Args...)
		for k, v := range e.serverConfig.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		e.session, err = e.client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	} else {
		e.session, err = e.client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: e.serverConfig.Url}, nil)
	}
	if err != nil {
		return err
	}

	return nil
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
			session:  e.session,
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
		if textContent, ok := content.(*mcp.TextContent); ok {
			builder.WriteString(textContent.Text)
		}
	}
	return builder.String(), nil
}

// McpTool 实现 tool.Tool 接口
type McpTool struct {
	toolName string // 给 mcp server 看的，和给模型看的不一样
	client   *McpClient
	session  *mcp.ClientSession
	mcpTool  *mcp.Tool
}

// ToolName 给模型看的，和给 mcp server 看的不一样
func (t *McpTool) ToolName() string {
	return fmt.Sprintf("babyagent_mcp__%s__%s", t.client.Name(), t.toolName)
}

func (t *McpTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared2.FunctionDefinitionParam{
		Description: openai.String(t.mcpTool.Description),
		Name:        t.ToolName(),
		Parameters:  t.mcpTool.InputSchema.(map[string]any),
	})
}

func (t *McpTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	return t.client.callTool(ctx, t.toolName, argumentsInJSON)
}
