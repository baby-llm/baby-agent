package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"

	"babyagent/ch11/agent"
	"babyagent/ch11/agent/tool"
	"babyagent/ch11/observe"
	"babyagent/ch11/server"
	"babyagent/shared"
	"babyagent/shared/log"
)

func main() {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := observe.InitTracer(ctx, "babyagent-ch11")
	if err != nil {
		log.Errorf("init tracer: %v", err)
		return
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			log.Errorf("shutdown tracer: %v", err)
		}
	}()

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
	defer func() {
		if err := server.CloseDB(db); err != nil {
			log.Errorf("close db: %v", err)
		}
	}()

	metrics := observe.NewMetrics(prometheus.DefaultRegisterer)
	a := agent.NewAgent(conf.LLMProviders.FrontModel, agent.SystemPrompt, []tool.Tool{tool.NewBashTool()}, metrics)
	router := server.NewRouter(server.NewServer(db, a), metrics)

	srv := &http.Server{Addr: ":8080", Handler: router}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("server failed: %v", err)
		}
	}()

	// 收到 SIGINT/SIGTERM 后给在途请求 5 秒收尾，再关闭数据库和 tracer。
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("server shutdown: %v", err)
	}
}
