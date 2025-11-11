package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/gitea-jenkins-webhook/internal/config"
	"github.com/example/gitea-jenkins-webhook/internal/gitea"
	"github.com/example/gitea-jenkins-webhook/internal/jenkins"
	"github.com/example/gitea-jenkins-webhook/internal/processor"
	"github.com/example/gitea-jenkins-webhook/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	jClient := jenkins.NewClient(cfg.Jenkins.BaseURL, cfg.Jenkins.Username, cfg.Jenkins.APIToken, cfg.Jenkins.JobTree, nil)
	gClient := gitea.NewClient(cfg.Gitea.BaseURL, cfg.Gitea.Token, nil)

	proc := processor.New(cfg, jClient, gClient, logger)
	srv := server.New(cfg, proc, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		logger.Error("server terminated with error", "err", err)
		os.Exit(1)
	}
}
