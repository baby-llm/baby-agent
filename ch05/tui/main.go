package main

import (
	"context"
	"io"
	"log"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/joho/godotenv"

	"babyagent/ch05"
	"babyagent/ch05/tool"
	"babyagent/shared"

	ctxengine "babyagent/ch05/context"
	"babyagent/ch05/storage"
)

func main() {
	_ = godotenv.Load()

	ctx := context.Background()
	appConf, err := shared.LoadAppConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to config.json: %v", err)
	}

	mcpServerMap, err := shared.LoadMcpServerConfig("mcp-server.json")
	if err != nil {
		log.Printf("Failed to load MCP server configuration: %v", err)
	}
	mcpClients := make([]*ch05.McpClient, 0)
	for k, v := range mcpServerMap {
		mcpClient := ch05.NewMcpToolProvider(k, v)
		if err := mcpClient.RefreshTools(ctx); err != nil {
			log.Printf("Failed to refresh tools for MCP server %s: %v", k, err)
			continue
		}
		mcpClients = append(mcpClients, mcpClient)
	}

	// 创建上下文引擎和 policy
	store := storage.NewMemoryStorage()
	summarizer := ctxengine.NewLLMSummarizer(appConf.LLMProviders.BackModel, 200)

	policies := []ctxengine.Policy{
		ctxengine.NewOffloadPolicy(store, 0.4, 0, 100),
		ctxengine.NewSummaryPolicy(summarizer, 10, 20, 0.6),
		ctxengine.NewTruncatePolicy(0, 0.85),
	}
	contextEngine := ctxengine.NewContextEngine(policies)

	agent := ch05.NewAgent(
		appConf.LLMProviders.FrontModel,
		ch05.CodingAgentSystemPrompt,
		[]tool.Tool{tool.NewBashTool(), tool.NewLoadStorageTool(store)},
		mcpClients,
		contextEngine,
	)

	log.SetOutput(io.Discard)
	p := tea.NewProgram(newModel(agent, appConf.LLMProviders.FrontModel.Model))
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
