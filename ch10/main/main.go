package main

import (
	"github.com/joho/godotenv"

	"babyagent/ch10/agent"
	"babyagent/ch10/agent/tool"
	"babyagent/ch10/server"
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

	db, err := server.InitDB("ch10.db")
	if err != nil {
		log.Errorf("Failed to initialize database: %v", err)
		panic(err)
	}
	defer func() {
		if err := server.CloseDB(db); err != nil {
			log.Errorf("Failed to close database: %v", err)
		}
	}()

	a := agent.NewAgent(appConf.LLMProviders.FrontModel, agent.SystemPrompt, []tool.Tool{tool.NewBashTool()})
	s := server.NewServer(db, a)
	router := server.NewRouter(s)

	if err := router.Run(":8080"); err != nil {
		log.Errorf("Server failed: %v", err)
		panic(err)
	}
}
