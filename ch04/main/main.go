package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/joho/godotenv"

	"babyagent/ch04"
	"babyagent/ch04/tool"
	"babyagent/ch04/tui"
	"babyagent/shared"
)

func main() {
	_ = godotenv.Load()

	modelConf := shared.NewModelConfig()

	// MCP 初始化可能拉起 npx/网络连接，给一个超时，避免启动阶段无限挂起。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	mcpServerMap, err := shared.LoadMcpServerConfig("mcp-server.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load mcp-server.json: %v\n", err)
	}
	mcpClients := make([]*ch04.McpClient, 0)
	for name, server := range mcpServerMap {
		client := ch04.NewMcpToolProvider(name, server)
		if err := client.RefreshTools(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "start MCP server %s: %v\n", name, err)
			_ = client.Close()
			continue
		}
		mcpClients = append(mcpClients, client)
	}
	cancel()
	defer func() {
		for _, client := range mcpClients {
			_ = client.Close()
		}
	}()

	agent := ch04.NewAgent(
		modelConf,
		ch04.CodingAgentSystemPrompt,
		[]tool.Tool{tool.NewBashTool()},
		mcpClients,
	)

	log.SetOutput(io.Discard)
	p := tea.NewProgram(tui.NewModel(agent, modelConf.Model))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI failed: %v\n", err)
		os.Exit(1)
	}
}
