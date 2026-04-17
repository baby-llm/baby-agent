package main

import (
	"github.com/joho/godotenv"

	"babyagent/ch12/agent"
	"babyagent/ch12/agent/tool"
	"babyagent/ch12/server"
	"babyagent/shared"
	"babyagent/shared/log"
)

func main() {
	_ = godotenv.Load()

	appConf, err := shared.LoadAppConfig("config.json")
	if err != nil {
		log.Errorf("Failed to load config.json: %v", err)
		panic(err)
	}

	db, err := server.InitDB("ch12.db")
	if err != nil {
		log.Errorf("Failed to initialize database: %v", err)
		panic(err)
	}

	a := agent.NewAgent(appConf.LLMProviders.FrontModel, agent.SystemPrompt, []tool.Tool{tool.NewBashTool()})
	s := server.NewServer(db, a)
	router := server.NewRouter(s)

	if err := router.Run(":8080"); err != nil {
		log.Errorf("Server failed: %v", err)
		panic(err)
	}
}
