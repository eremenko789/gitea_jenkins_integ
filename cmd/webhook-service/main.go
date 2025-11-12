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
	debugFlag := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	logLevel := slog.LevelInfo
	if *debugFlag {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting webhook service", "config_path", *configPath, "debug", *debugFlag)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	logger.Info("configuration loaded successfully",
		"server_addr", cfg.Server.ListenAddr,
		"worker_pool_size", cfg.Server.WorkerPoolSize,
		"queue_size", cfg.Server.QueueSize,
		"repositories_count", len(cfg.Repositories))

	jClient := jenkins.NewClient(cfg.Jenkins.BaseURL, cfg.Jenkins.Username, cfg.Jenkins.APIToken, nil, logger)
	gClient := gitea.NewClient(cfg.Gitea.BaseURL, cfg.Gitea.Token, nil, logger)

	logger.Info("initializing processor and server")
	proc := processor.New(cfg, jClient, gClient, logger)
	srv := server.New(cfg, proc, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("webhook service started successfully")
	if err := srv.Run(ctx); err != nil {
		logger.Error("server terminated with error", "err", err)
		os.Exit(1)
	}
	logger.Info("webhook service stopped")
}
