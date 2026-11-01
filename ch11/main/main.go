package main

import (
	"babyagent/ch11/agent"
	"babyagent/ch11/agent/tool"
	"babyagent/ch11/observe"
	"babyagent/ch11/server"
	"babyagent/shared"
	"babyagent/shared/log"
	"context"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"os/signal"
	"syscall"
)

func main() {
	_ = godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdown, err := observe.InitTracer(ctx, "babyagent-ch11")
	if err != nil {
		panic(err)
	}
	defer shutdown(context.Background())
	conf, err := shared.LoadAppConfig("config.json")
	if err != nil {
		log.Errorf("load config: %v", err)
		return
	}
	db, err := server.InitDB("ch11.db")
	if err != nil {
		log.Errorf("init db: %v", err)
		return
	}
	metrics := observe.NewMetrics(prometheus.DefaultRegisterer)
	a := agent.NewAgent(conf.LLMProviders.FrontModel, agent.SystemPrompt, []tool.Tool{tool.NewBashTool()}, metrics)
	if err := server.NewRouter(server.NewServer(db, a), metrics).Run(":8080"); err != nil {
		log.Errorf("server failed: %v", err)
	}
}
