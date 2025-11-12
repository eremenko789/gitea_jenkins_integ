package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/gitea-jenkins-webhook/internal/config"
	"github.com/example/gitea-jenkins-webhook/internal/gitea"
	"github.com/example/gitea-jenkins-webhook/internal/jenkins"
	"github.com/example/gitea-jenkins-webhook/internal/processor"
	"github.com/example/gitea-jenkins-webhook/internal/server"
)

// runCommand запускает вебхук-сервис. Загружает конфигурацию, инициализирует клиенты
// для работы с Jenkins и Gitea, создает процессор и сервер, затем запускает сервер
// и обрабатывает сигналы завершения для корректного завершения работы.
func runCommand() {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "Path to configuration file")
	debugFlag := fs.Bool("debug", false, "Enable debug logging")
	fs.Parse(os.Args[1:])

	logger := setupLogger(*debugFlag)

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
