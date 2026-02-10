package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/vango-go/vango"
	"rhone_chat/app/routes"
	"rhone_chat/internal/ai"
	"rhone_chat/internal/config"
	"rhone_chat/internal/db"
	chatsvc "rhone_chat/internal/services/chat"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	store, err := db.OpenSQLite(cfg.DatabasePath)
	if err != nil {
		slog.Error("failed to open sqlite store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	runner := ai.NewRunner(ai.RunnerConfig{
		MaxTurns:     cfg.MaxTurns,
		MaxToolCalls: cfg.MaxToolCalls,
		RunTimeout:   cfg.RunTimeout,
		ToolTimeout:  cfg.ToolTimeout,
	})
	chatService := chatsvc.NewService(store, runner, cfg)

	app, err := vango.New(vango.Config{
		Session: vango.SessionConfig{
			ResumeWindow: vango.ResumeWindow(30 * time.Second),
		},
		Static: vango.StaticConfig{
			Dir:    "public",
			Prefix: "/",
		},
		DevMode: cfg.DevMode,
	})
	if err != nil {
		slog.Error("failed to create app", "error", err)
		os.Exit(1)
	}

	routes.SetDeps(routes.Deps{
		Chat: chatService,
	})
	routes.Register(app)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr)
	if err := app.Run(ctx, addr); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
