package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/joho/godotenv"

	"babyagent/ch08"
	ctxengine "babyagent/ch08/context"
	"babyagent/ch08/memory"
	"babyagent/ch08/storage"
	"babyagent/ch08/tool"
	"babyagent/ch08/tui"
	"babyagent/shared"
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
	mcpClients := make([]*ch08.McpClient, 0)
	for k, v := range mcpServerMap {
		mcpClient := ch08.NewMcpToolProvider(k, v)
		if err := mcpClient.RefreshTools(ctx); err != nil {
			log.Printf("Failed to refresh tools for MCP server %s: %v", k, err)
			continue
		}
		mcpClients = append(mcpClients, mcpClient)
	}

	// 创建上下文引擎和 policy
	memoryStorage := storage.NewMemoryStorage()
	summarizer := ctxengine.NewLLMSummarizer(appConf.LLMProviders.BackModel, 200)

	policies := []ctxengine.Policy{
		ctxengine.NewOffloadPolicy(memoryStorage, 0.4, 0, 100),
		ctxengine.NewSummaryPolicy(summarizer, 10, 20, 0.6),
		ctxengine.NewTruncatePolicy(0, 0.85),
	}

	homeStorage := storage.NewFileSystemStorage(filepath.Join(shared.GetHomeDir(), ".babyagent"))
	workspaceStorage := storage.NewFileSystemStorage(filepath.Join(shared.GetWorkspaceDir(), ".babyagent"))
	memoryUpdater := memory.NewLLMMemoryUpdater(appConf.LLMProviders.BackModel)
	multiLevelMemory, err := memory.NewMultiLevelMemory(homeStorage, workspaceStorage, memoryUpdater)
	if err != nil {
		log.Fatalf("Failed to load memory: %v", err)
	}

	contextEngine := ctxengine.NewContextEngine(multiLevelMemory, policies)

	// 配置需要确认的工具
	confirmConfig := ch08.ToolConfirmConfig{
		RequireConfirmTools: map[tool.AgentTool]bool{
			tool.AgentToolBash: true, // bash 命令需要确认
		},
	}

	bashTool, bashWarn := tool.CreateBashTool(shared.GetWorkspaceDir())
	if bashWarn != nil {
		log.Printf("WARNING: %v", bashWarn)
	}

	agent := ch08.NewAgent(
		appConf.LLMProviders.FrontModel,
		ch08.CodingAgentSystemPrompt,
		confirmConfig,
		[]tool.Tool{bashTool, tool.NewLoadStorageTool(memoryStorage)},
		mcpClients,
		contextEngine,
	)

	log.SetOutput(io.Discard)
	p := tea.NewProgram(tui.NewModel(agent, appConf.LLMProviders.FrontModel.Model))
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
